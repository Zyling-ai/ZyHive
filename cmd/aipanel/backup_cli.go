package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/backup"
)

func backupCommandArgs(args []string) ([]string, string, bool, error) {
	configPath := os.Getenv("AIPANEL_CONFIG")
	if configPath == "" {
		configPath = "aipanel.json"
	}
	clean := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 >= len(args) {
				return nil, "", false, errors.New("--config requires a path")
			}
			configPath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
			configPath = strings.SplitN(arg, "=", 2)[1]
			continue
		}
		clean = append(clean, arg)
	}
	if len(clean) == 0 || clean[0] != "backup" {
		return nil, configPath, false, nil
	}
	return clean[1:], configPath, true, nil
}

func runBackupCLI(args []string, configPath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printBackupHelp()
		return nil
	}
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current work directory: %w", err)
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("backup create", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		output := fs.String("output", "", "output .tar.gz path")
		work := fs.String("workdir", workDir, "runtime work directory")
		cfg := fs.String("config", configPath, "current config path")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
		}
		if *output == "" {
			return errors.New("backup create requires --output")
		}
		manifest, err := backup.Create(backup.CreateOptions{
			Output: *output, ConfigPath: *cfg, WorkDir: *work, AppVersion: Version,
		})
		if err != nil {
			return err
		}
		fmt.Printf("backup created: %s (%d entries)\n", *output, len(manifest.Entries))
		return nil
	case "inspect":
		fs := flag.NewFlagSet("backup inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		input := fs.String("input", "", "input .tar.gz path")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
		}
		if *input == "" {
			return errors.New("backup inspect requires --input")
		}
		manifest, err := backup.Inspect(*input, backup.Limits{})
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "restore":
		fs := flag.NewFlagSet("backup restore", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		input := fs.String("input", "", "input .tar.gz path")
		work := fs.String("workdir", workDir, "runtime work directory")
		cfg := fs.String("config", configPath, "current config path")
		yes := fs.Bool("yes", false, "confirm destructive restore")
		noService := fs.Bool("no-service", false, "do not stop/start the ZyHive service")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
		}
		if *input == "" {
			return errors.New("backup restore requires --input")
		}
		if !*yes {
			return errors.New("backup restore requires --yes")
		}
		// Validate before touching service state. Restore validates again before staging.
		if _, err := backup.Inspect(*input, backup.Limits{}); err != nil {
			return err
		}
		serviceStopped := false
		if !*noService {
			if err := controlBackupService("stop"); err != nil {
				return fmt.Errorf("stop service (use --no-service when externally managed): %w", err)
			}
			serviceStopped = true
		}
		_, restoreErr := backup.Restore(backup.RestoreOptions{
			Input: *input, ConfigPath: *cfg, WorkDir: *work,
		})
		var startErr error
		if serviceStopped {
			startErr = controlBackupService("start")
		}
		if restoreErr != nil || startErr != nil {
			return errors.Join(restoreErr, func() error {
				if startErr != nil {
					return fmt.Errorf("restart service: %w", startErr)
				}
				return nil
			}())
		}
		fmt.Println("backup restored successfully")
		return nil
	default:
		return fmt.Errorf("unknown backup command %q", args[0])
	}
}

func controlBackupService(action string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("launchctl", action, "com.zyhive.zyhive")
	case "windows":
		cmd = exec.Command("sc", action, "zyhive")
	default:
		cmd = exec.Command("systemctl", action, "zyhive")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func interactiveBackupPaths() (configPath, workDir string, err error) {
	configPath = findConfigPath()
	workDir, err = os.Getwd()
	if err != nil {
		return "", "", err
	}
	configPath, err = filepath.Abs(configPath)
	return configPath, workDir, err
}

func printBackupHelp() {
	fmt.Print(`ZyHive versioned backup

Usage:
  zyhive backup create  --output FILE [--config FILE] [--workdir DIR]
  zyhive backup inspect --input FILE
  zyhive backup restore --input FILE --yes [--no-service] [--config FILE] [--workdir DIR]
`)
}
