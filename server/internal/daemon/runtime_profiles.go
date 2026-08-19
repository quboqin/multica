package daemon

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/pkg/agent"
)

var (
	lookPath              = exec.LookPath
	profilePathExecutable = func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
)

var ErrNoRuntimesToRegister = errors.New("no agent runtimes could be registered")

type profileLaunchSpec struct {
	path      string
	version   string
	fixedArgs []string
}

func (d *Daemon) recordProfileLaunch(profileID, path, version string, fixedArgs []string) {
	if profileID == "" || path == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.profileLaunchSpecs == nil {
		d.profileLaunchSpecs = make(map[string]profileLaunchSpec)
	}
	d.profileLaunchSpecs[profileID] = profileLaunchSpec{path: path, version: version, fixedArgs: append([]string(nil), fixedArgs...)}
}

func (d *Daemon) customProfileLaunchForRuntime(runtimeID string) (profileLaunchSpec, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	runtime, ok := d.runtimeIndex[runtimeID]
	if !ok || runtime.ProfileID == "" {
		return profileLaunchSpec{}, false
	}
	spec, ok := d.profileLaunchSpecs[runtime.ProfileID]
	if !ok || spec.path == "" {
		return profileLaunchSpec{}, false
	}
	spec.fixedArgs = append([]string(nil), spec.fixedArgs...)
	return spec, true
}

func (d *Daemon) appendProfileRuntimes(ctx context.Context, workspaceID string, allowed map[string]struct{}, runtimes *[]map[string]any, failures *[]map[string]any) string {
	resp, err := d.client.GetRuntimeProfiles(ctx, workspaceID)
	if err != nil {
		d.logger.Info("skip custom runtime profiles: fetch failed", "workspace_id", workspaceID, "error", err)
		return ""
	}
	if resp == nil {
		return profileSetSignature(nil)
	}
	for _, profile := range resp.RuntimeProfiles {
		if profile.ID == "" || profile.CommandName == "" || profile.ProtocolFamily == "" {
			continue
		}
		if len(allowed) > 0 {
			if _, enabled := allowed[profile.ProtocolFamily]; !enabled {
				continue
			}
		}
		if !agent.IsSupportedType(profile.ProtocolFamily) {
			*failures = append(*failures, map[string]any{"profile_id": profile.ID, "command_name": profile.CommandName, "reason": "unsupported protocol_family: " + profile.ProtocolFamily})
			continue
		}
		resolved := ""
		failureReason := ""
		if override := strings.TrimSpace(d.cfg.ProfileCommandOverrides[profile.ID]); override != "" {
			if profilePathExecutable(override) {
				resolved = override
			} else {
				failureReason = "configured path override is not executable: " + override
			}
		}
		if resolved == "" {
			path, lookupErr := lookPath(profile.CommandName)
			if lookupErr != nil {
				if failureReason != "" {
					failureReason += "; "
				}
				failureReason += "command not found on PATH: " + profile.CommandName
				*failures = append(*failures, map[string]any{"profile_id": profile.ID, "command_name": profile.CommandName, "reason": failureReason})
				continue
			}
			resolved = path
		}
		version, versionErr := detectAgentVersion(ctx, resolved)
		if versionErr != nil {
			version = ""
		}
		name := profile.DisplayName
		if d.cfg.DeviceName != "" {
			name = fmt.Sprintf("%s (%s)", name, d.cfg.DeviceName)
		}
		d.recordProfileLaunch(profile.ID, resolved, version, profile.FixedArgs)
		*runtimes = append(*runtimes, map[string]any{"name": name, "type": profile.ProtocolFamily, "version": version, "status": "online", "profile_id": profile.ID})
	}
	return profileSetSignature(resp.RuntimeProfiles)
}

func profileSetSignature(profiles []RuntimeProfile) string {
	if len(profiles) == 0 {
		return "0"
	}
	sorted := append([]RuntimeProfile(nil), profiles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	h := fnv.New64a()
	for _, profile := range sorted {
		fmt.Fprintf(h, "%s\x1f%t\x1f%s\x1f%s\x1f%s\x1f", profile.ID, profile.Enabled, profile.ProtocolFamily, profile.CommandName, profile.Visibility)
		for _, arg := range profile.FixedArgs {
			fmt.Fprintf(h, "%s\x1f", arg)
		}
		_, _ = h.Write([]byte("\x1e"))
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// refreshWorkspaceRuntimeProfiles converges a running daemon after an admin
// changes the workspace profile set. It deliberately replaces only this
// workspace's runtime index and never calls orphan recovery: adding a sibling
// profile must not interrupt a task already running on a built-in runtime.
func (d *Daemon) refreshWorkspaceRuntimeProfiles(ctx context.Context, workspaceID string) error {
	d.mu.Lock()
	workspace, tracked := d.workspaces[workspaceID]
	if !tracked {
		d.mu.Unlock()
		return nil
	}
	settings := append([]byte(nil), workspace.settings...)
	currentSignature := workspace.profileSetSig
	d.mu.Unlock()

	profiles, err := d.client.GetRuntimeProfiles(ctx, workspaceID)
	if err != nil {
		return err
	}
	var live []RuntimeProfile
	if profiles != nil {
		live = profiles.RuntimeProfiles
	}
	if signature := profileSetSignature(live); signature == currentSignature {
		return nil
	}

	registered, signature, err := d.registerRuntimesForWorkspace(ctx, workspaceID, settings)
	if err != nil {
		if !errors.Is(err, ErrNoRuntimesToRegister) {
			return err
		}
		d.mu.Lock()
		workspace, stillTracked := d.workspaces[workspaceID]
		if !stillTracked {
			d.mu.Unlock()
			return nil
		}
		removed := append([]string(nil), workspace.runtimeIDs...)
		for _, runtimeID := range removed {
			delete(d.runtimeIndex, runtimeID)
		}
		workspace.runtimeIDs = nil
		workspace.profileSetSig = signature
		d.mu.Unlock()
		if len(removed) > 0 {
			if err := d.client.Deregister(ctx, removed); err != nil {
				d.logger.Warn("deregister runtimes after profile removal failed", "workspace_id", workspaceID, "error", err)
			}
		}
		d.notifyRuntimeSetChanged()
		return nil
	}

	newIDs := make([]string, 0, len(registered.Runtimes))
	newSet := make(map[string]struct{}, len(registered.Runtimes))
	for _, runtime := range registered.Runtimes {
		newIDs = append(newIDs, runtime.ID)
		newSet[runtime.ID] = struct{}{}
	}
	d.mu.Lock()
	workspace, tracked = d.workspaces[workspaceID]
	if !tracked {
		d.mu.Unlock()
		return nil
	}
	removed := make([]string, 0)
	for _, runtimeID := range workspace.runtimeIDs {
		if _, kept := newSet[runtimeID]; !kept {
			delete(d.runtimeIndex, runtimeID)
			removed = append(removed, runtimeID)
		}
	}
	for _, runtime := range registered.Runtimes {
		d.runtimeIndex[runtime.ID] = runtime
	}
	workspace.runtimeIDs = newIDs
	workspace.profileSetSig = signature
	if registered.ReposVersion != "" {
		workspace.reposVersion = registered.ReposVersion
		workspace.allowedRepoURLs = repoAllowlist(registered.Repos)
	}
	if len(registered.Settings) > 0 {
		workspace.settings = registered.Settings
	}
	d.mu.Unlock()
	if len(removed) > 0 {
		if err := d.client.Deregister(ctx, removed); err != nil {
			d.logger.Warn("deregister runtimes after profile refresh failed", "workspace_id", workspaceID, "error", err)
		}
	}
	d.notifyRuntimeSetChanged()
	return nil
}
