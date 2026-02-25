package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"syl-listing/internal/app"
	"syl-listing/internal/config"
)

type genFlags struct {
	configArg      string
	outputDirArg   string
	numArg         int
	concurrencyArg int
	maxRetriesArg  int
	providerArg    string
	logFileArg     string
	verboseArg     bool
}

var loadConfigForUpdate = config.Load
var syncRulesForUpdate = config.SyncRulesFromCenter

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
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "显示版本信息")

	genCmd := &cobra.Command{
		Use:           "gen [file_or_dir ...]",
		Short:         "生成 listing Markdown 文件",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runGen(stdout, stderr, flags, true, &showVersion),
	}
	bindGenFlags(genCmd, flags)
	genCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "显示版本信息")
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

	setCmd := &cobra.Command{
		Use:           "set",
		Short:         "设置本地参数",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	setKeyCmd := &cobra.Command{
		Use:                   "key <api_key>",
		Short:                 "设置 API KEY",
		Args:                  cobra.ExactArgs(1),
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			if key == "" {
				return fmt.Errorf("API Key 不能为空")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("读取当前目录失败：%w", err)
			}
			_, paths, err := config.Load(flags.configArg, cwd)
			if err != nil {
				return err
			}
			return config.UpsertEnvVar(paths.EnvPath, "DEEPSEEK_API_KEY", key)
		},
	}
	setCmd.AddCommand(setKeyCmd)
	root.AddCommand(setCmd)

	updateCmd := &cobra.Command{
		Use:           "update",
		Short:         "更新本地资源",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	updateRulesCmd := &cobra.Command{
		Use:           "rules",
		Short:         "清除本地规则缓存并重新下载最新规则",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("读取当前目录失败：%w", err)
			}
			msg, err := runUpdateRules(flags.configArg, cwd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(msg) != "" {
				fmt.Fprintln(stdout, msg)
			}
			return nil
		},
	}
	updateCmd.AddCommand(updateRulesCmd)
	root.AddCommand(updateCmd)
	return root
}

func bindGenFlags(cmd *cobra.Command, flags *genFlags) {
	cmd.Flags().StringVar(&flags.configArg, "config", "", "配置文件路径，默认 ~/.syl-listing/config.yaml")
	cmd.Flags().StringVarP(&flags.outputDirArg, "out", "o", "", "输出目录，默认当前目录")
	cmd.Flags().IntVarP(&flags.numArg, "num", "n", 0, "每个需求文件生成候选数量")
	cmd.Flags().IntVar(&flags.concurrencyArg, "concurrency", 0, "保留参数（当前版本不限制并发）")
	cmd.Flags().IntVar(&flags.maxRetriesArg, "max-retries", 0, "最大重试次数")
	cmd.Flags().StringVar(&flags.providerArg, "provider", "", "覆盖配置中的 provider（当前仅支持 deepseek）")
	cmd.Flags().StringVar(&flags.logFileArg, "log-file", "", "NDJSON 日志文件路径")
	cmd.Flags().BoolVar(&flags.verboseArg, "verbose", false, "输出详细 NDJSON（机器友好）")
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
			Verbose:         flags.verboseArg,
			CWD:             cwd,
			Stdout:          stdout,
			Stderr:          stderr,
			Stdin:           os.Stdin,
			InvokedSubcmd:   subcommand,
			NormalizedInput: normalizeArgs(os.Args[1:]),
		})
		if err != nil {
			return err
		}

		finalLine := fmt.Sprintf(
			"任务完成：成功 %d，失败 %d，总耗时 %s，余额：%s",
			res.Succeeded,
			res.Failed,
			formatDurationMS(res.ElapsedMS),
			formatSummaryBalance(res.Balance),
		)
		if res.Failed > 0 {
			return fmt.Errorf(finalLine)
		}
		if !flags.verboseArg {
			fmt.Fprintln(stdout, finalLine)
		}
		return nil
	}
}

func formatDurationMS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.2fs", float64(ms)/1000.0)
	}
	minutes := ms / 60_000
	remainMS := ms % 60_000
	if remainMS == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm%.1fs", minutes, float64(remainMS)/1000.0)
}

func formatSummaryBalance(balance string) string {
	if strings.TrimSpace(balance) == "" {
		return "查询失败"
	}
	return strings.TrimSpace(balance)
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	switch first {
	case "gen", "help", "completion", "version", "set", "update":
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

func runUpdateRules(configArg, cwd string) (string, error) {
	cfg, paths, err := loadConfigForUpdate(configArg, cwd)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(paths.ResolvedRulesDir); err != nil {
		return "", fmt.Errorf("清除本地规则缓存失败：%w", err)
	}
	if err := os.Remove(paths.RulesLockPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("清除规则锁失败：%w", err)
	}
	cfg.RulesCenter.Release = "latest"
	cfg.RulesCenter.Strict = true
	res, err := syncRulesForUpdate(cfg, paths)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Warning) != "" {
		return "", fmt.Errorf(strings.TrimSpace(res.Warning))
	}
	msg := strings.TrimSpace(res.Message)
	if msg == "" {
		msg = "规则更新完成"
	}
	if tag := extractRulesTag(msg); tag != "" {
		return tag, nil
	}
	return msg, nil
}

func extractRulesTag(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	l := strings.LastIndex(msg, "（")
	r := strings.LastIndex(msg, "）")
	if l >= 0 && r > l+1 {
		return strings.TrimSpace(msg[l+len("（") : r])
	}
	l = strings.LastIndex(msg, "(")
	r = strings.LastIndex(msg, ")")
	if l >= 0 && r > l+1 {
		return strings.TrimSpace(msg[l+1 : r])
	}
	return ""
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
