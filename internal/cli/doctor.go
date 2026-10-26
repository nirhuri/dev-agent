package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nirhuri/dev-agent/internal/config"
	"github.com/nirhuri/dev-agent/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Validate configuration, auth, and prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile, _ := cmd.Flags().GetString("config")
			cfg, source, err := config.Load(config.LoadOptions{ConfigFile: configFile})
			if err != nil {
				exists, exErr := config.Exists(configFile)
				if exErr != nil {
					return exErr
				}
				if !exists {
					cfg = config.Default()
					source = "defaults (no config file)"
				} else {
					return err
				}
			}

			report := doctor.Run(cmd.Context(), cfg, source)
			if cmd.Context() == nil {
				report = doctor.Run(context.Background(), cfg, source)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Dev Agent doctor")
			fmt.Fprintln(cmd.OutOrStdout())
			for _, r := range report.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", r.Severity, r.Name, r.Message)
				if r.Fix != "" && r.Severity != doctor.SeverityOK {
					fmt.Fprintf(cmd.OutOrStdout(), "       fix: %s\n", r.Fix)
				}
			}

			if report.HasErrors() {
				return fmt.Errorf("doctor found errors")
			}
			return nil
		},
	}
}
