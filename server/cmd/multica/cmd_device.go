package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/deviceruntime"
)

func newDeviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "device", Short: "Run a local Android Device Runtime"}
	serve := &cobra.Command{Use: "serve", Short: "Serve the local device control console", Args: cobra.NoArgs, RunE: runDeviceServe}
	serve.Flags().String("adb", "adb", "Path to adb executable")
	serve.Flags().String("artifact", "", "APK artifact path (required)")
	serve.Flags().String("package", "", "Android package name (required)")
	serve.Flags().String("activity", "", "Launch activity component (required)")
	serve.Flags().String("scrcpy-server", "", "Path to the matching scrcpy-server binary (required for WebRTC video)")
	serve.Flags().String("scrcpy-version", "3.3.4", "scrcpy server protocol version")
	serve.Flags().String("access-token", os.Getenv("MULTICA_DEVICE_RUNTIME_TOKEN"), "Host access token for native Device Runtime clients")
	serve.Flags().Int("port", 18080, "Loopback port for the local console")
	_ = serve.MarkFlagRequired("artifact")
	_ = serve.MarkFlagRequired("package")
	_ = serve.MarkFlagRequired("activity")
	cmd.AddCommand(serve)
	return cmd
}

func runDeviceServe(cmd *cobra.Command, _ []string) error {
	artifact, _ := cmd.Flags().GetString("artifact")
	artifact, err := filepath.Abs(artifact)
	if err != nil {
		return err
	}
	if _, err := os.Stat(artifact); err != nil {
		return fmt.Errorf("artifact: %w", err)
	}
	adb, _ := cmd.Flags().GetString("adb")
	packageName, _ := cmd.Flags().GetString("package")
	activity, _ := cmd.Flags().GetString("activity")
	scrcpyServer, _ := cmd.Flags().GetString("scrcpy-server")
	scrcpyVersion, _ := cmd.Flags().GetString("scrcpy-version")
	accessToken, _ := cmd.Flags().GetString("access-token")
	port, _ := cmd.Flags().GetInt("port")
	server := deviceruntime.Server{
		ADB:      deviceruntime.ADB{Binary: adb},
		Artifact: artifact, Package: packageName, Component: activity,
		ScrcpyServer: scrcpyServer, ScrcpyVersion: scrcpyVersion,
		AccessToken: accessToken,
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Fprintf(cmd.OutOrStdout(), "Multica Device Runtime listening at http://%s\n", address)
	return http.ListenAndServe(address, server.Handler())
}

var deviceCmd = newDeviceCommand()
