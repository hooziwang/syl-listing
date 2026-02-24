package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"syl-listing/internal/app"
)

type genFlags struct {
	configArg      string
	outputDirArg   string
	numArg         int
	concurrencyArg int
	maxRetriesArg  int
	providerArg    string
	logFileArg     string
}

func Execute() error {
	root := NewRootCmd(os.Stdout, os.Stderr)
	root.SetArgs(normalizeArgs(os.Args[1:]))
	err := root.Execute()
	if err == errAlreadyPrinted {
		return nil
	}
	return err
}

func NewRootCmd(stdout, stderr *os.File) *cobra.Command {
	flags := &genFlags{}
	showVersion := false

	root := &cobra.Command{
		Use:           "syl-listing [file_or_dir ...]",
		Short:         "根据 listing 需求文件批量生成中英 Markdown",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runGen(stdout, stderr, flags, false, &showVersion),
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.HiddenDefaultCmd = true
	bindGenFlags(root, flags)
	root.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "显示版本信息")

	genCmd := &cobra.Command{
		Use:           "gen [file_or_dir ...]",
		Short:         "生成 listing Markdown 文件",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runGen(stdout, stderr, flags, true, &showVersion),
	}
	root.AddCommand(genCmd)

	versionCmd := &cobra.Command{
		Use:           "version",
		Short:         "显示版本信息",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			printVersion(stdout)
		},
	}
	root.AddCommand(versionCmd)
	return root
}

func bindGenFlags(cmd *cobra.Command, flags *genFlags) {
	cmd.PersistentFlags().StringVar(&flags.configArg, "config", "", "配置文件路径，默认 ~/.syl-listing/config.yaml")
	cmd.PersistentFlags().StringVarP(&flags.outputDirArg, "out", "o", "", "输出目录，默认当前目录")
	cmd.PersistentFlags().IntVarP(&flags.numArg, "num", "n", 0, "每个需求文件生成候选数量")
	cmd.PersistentFlags().IntVar(&flags.concurrencyArg, "concurrency", 0, "保留参数（当前版本不限制并发）")
	cmd.PersistentFlags().IntVar(&flags.maxRetriesArg, "max-retries", 0, "最大重试次数")
	cmd.PersistentFlags().StringVar(&flags.providerArg, "provider", "", "覆盖配置中的 provider")
	cmd.PersistentFlags().StringVar(&flags.logFileArg, "log-file", "", "NDJSON 日志文件路径")
}

func runGen(stdout, stderr *os.File, flags *genFlags, subcommand bool, showVersion *bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if showVersion != nil && *showVersion {
			printVersion(stdout)
			return nil
		}

		if len(args) == 0 {
			_ = cmd.Help()
			return nil
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("读取当前目录失败：%w", err)
		}

		res, err := app.Run(app.Options{
			Inputs:          args,
			ConfigPath:      flags.configArg,
			OutputDir:       flags.outputDirArg,
			Num:             flags.numArg,
			Concurrency:     flags.concurrencyArg,
			MaxRetries:      flags.maxRetriesArg,
			Provider:        flags.providerArg,
			LogFile:         flags.logFileArg,
			CWD:             cwd,
			Stdout:          stdout,
			Stderr:          stderr,
			InvokedSubcmd:   subcommand,
			NormalizedInput: normalizeArgs(os.Args[1:]),
		})
		if err != nil {
			return err
		}

		if res.Failed > 0 {
			return fmt.Errorf("任务完成：成功 %d，失败 %d", res.Succeeded, res.Failed)
		}
		fmt.Fprintf(stdout, "任务完成：成功 %d，失败 %d\n", res.Succeeded, res.Failed)
		return nil
	}
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	switch first {
	case "gen", "help", "completion", "version":
		return args
	}
	if first == "-h" || first == "--help" || first == "-v" || first == "--version" {
		return args
	}
	if !containsPositionalSource(args) {
		return args
	}
	return append([]string{"gen"}, args...)
}

func containsPositionalSource(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return i+1 < len(args)
		}
		if arg == "--config" || arg == "--out" || arg == "-o" || arg == "--num" || arg == "-n" || arg == "--concurrency" || arg == "--max-retries" || arg == "--provider" || arg == "--log-file" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--out=") || strings.HasPrefix(arg, "--num=") || strings.HasPrefix(arg, "--concurrency=") || strings.HasPrefix(arg, "--max-retries=") || strings.HasPrefix(arg, "--provider=") || strings.HasPrefix(arg, "--log-file=") {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return true
	}
	return false
}
