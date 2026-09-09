package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/pkg/imagemodel"
	"github.com/spf13/cobra"
)

var imageSettingsCmd = &cobra.Command{
	Use: "settings", Short: "Read frozen image model and quality from an order, task, or operation JSON file",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, _ := cmd.Flags().GetString("input-file")
		if path == "" {
			return fmt.Errorf("--input-file is required")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		settings, err := imageSettingsFromFile(raw)
		if err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, settings)
	},
}

func init() {
	imageCmd.AddCommand(imageSettingsCmd)
	imageSettingsCmd.Flags().String("input-file", "", "Order, task context, or image operation JSON file")
}

func imageSettingsFromFile(raw []byte) (imagemodel.Settings, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return imagemodel.Settings{}, fmt.Errorf("image settings input must be an object")
	}
	if snapshot, exists := envelope["input_snapshot"]; exists {
		return imagemodel.FromSnapshot(snapshot)
	}
	if context, exists := envelope["context"]; exists {
		return imagemodel.FromSnapshot(context)
	}
	return imagemodel.FromSnapshot(raw)
}
