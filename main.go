package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"essen/internal/body"
	"essen/internal/config"
	"essen/internal/models"
	"essen/internal/nutrition"
	"essen/internal/storage"
	"essen/internal/tui"
)

// ---------------------------------------------------------------------------
// ANSI escape helpers
// ---------------------------------------------------------------------------

var (
	colorEnabled bool
)

func init() {
	colorEnabled = os.Getenv("NO_COLOR") == "" &&
		os.Getenv("TERM") != "" &&
		os.Getenv("TERM") != "dumb" &&
		isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ansi returns the escape sequence when color is enabled, otherwise "".
func ansi(seq string) string {
	if !colorEnabled {
		return ""
	}
	return seq
}

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
)

// splitArgs separates positional arguments (before any flag) from flag
// arguments. This allows natural usage like "essen edit 1 --calories 200".
func splitArgs(args []string) (positional []string, flags []string) {
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "add":
		cmdAdd(os.Args[2:])
	case "list", "ls":
		cmdList(os.Args[2:])
	case "edit":
		cmdEdit(os.Args[2:])
	case "delete", "rm":
		cmdDelete(os.Args[2:])
	case "stats":
		cmdStats(os.Args[2:])
	case "today":
		cmdToday()
	case "config":
		cmdConfig(os.Args[2:])
	case "weight":
		cmdWeight(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "%s未知命令:%s %s\n\n", ansi(cRed), ansi(cReset), os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	exe := os.Args[0]
	fmt.Printf(`Essen — 饮食记录 CLI 工具（面向健身增肌）

用法:
  %s add [食物名] [--brand 品牌 --amount 份量 --time HH:MM] [-y]
      自动拆分: "A+B", "A和B", "A、B" → 分别查询并保存
  %s list [--date YYYY-MM-DD]                  列出饮食记录
  %s today                                     今日饮食列表 + 目标进度
  %s edit <序号> [选项...]                      编辑饮食记录
  %s delete <序号> [--date YYYY-MM-DD]         删除饮食记录
  %s stats [--date YYYY-MM-DD] [--week]        查看统计与目标进度
  %s config [选项...]                           查看/设置 LLM 配置
  %s weight [kg] [--list] [--xiaomi] [--config] 体重体测管理

编辑选项:
  --brand, --food, --amount, --calories, --protein, --fat, --carbs, --notes
  --date YYYY-MM-DD

配置选项:
  --llm-provider, --llm-model, --base-url, --api-key
  --xiaomi-user-id, --xiaomi-password, --height, --birth-date, --gender

数据存储: ~/.local/share/essen/
配置文件: ~/.config/essen/config.json
`, exe, exe, exe, exe, exe, exe, exe, exe)
}

// ---------------------------------------------------------------------------
// add
// ---------------------------------------------------------------------------

func cmdAdd(args []string) {
	pos, flagArgs := splitArgs(args)

	fs := flag.NewFlagSet("add", flag.ExitOnError)
	brand := fs.String("brand", "", "品牌 (可选)")
	food := fs.String("food", "", "食物名称")
	amount := fs.String("amount", "", "份量")
	timeStr := fs.String("time", "", "时间 HH:MM (默认现在)")
	yes := fs.Bool("y", false, "跳过确认")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen add [食物名] [--brand 品牌 --amount 份量 --time HH:MM] [-y]\n")
	}
	fs.Parse(flagArgs)

	// Positional args override --food if not explicitly set.
	foodStr := *food
	if foodStr == "" && len(pos) > 0 {
		foodStr = strings.Join(pos, " ")
	}

	// Interactive mode: only when no positional arg AND no --food flag (user hasn't provided anything).
	useInteractive := isTTY(os.Stdin) && foodStr == "" && *food == "" && len(pos) == 0

	if useInteractive {
		input, err := tui.ReadLine("  品牌 (可选): ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		*brand = strings.TrimSpace(input)

		input, err = tui.ReadLine("  食物名称: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		foodStr = strings.TrimSpace(input)

		input, err = tui.ReadLine("  份量: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		*amount = strings.TrimSpace(input)

		now := time.Now()
		input, err = tui.ReadLine(fmt.Sprintf("  时间 [HH:MM] (回车=现在 %s): ", now.Format("15:04")))
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		*timeStr = strings.TrimSpace(input)
	}

	// Set defaults for amount.
	if *amount == "" {
		*amount = "1份"
	}

	if foodStr == "" {
		fmt.Fprintf(os.Stderr, "%s错误:%s 食物名称不能为空\n", ansi(cRed), ansi(cReset))
		os.Exit(1)
	}

	// Validate time.
	recordTime := time.Now()
	if *timeStr != "" {
		parsed, err := time.Parse("15:04", *timeStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s 时间格式无效，请使用 HH:MM\n", ansi(cRed), ansi(cReset))
			os.Exit(1)
		}
		recordTime = parsed
	}

	// --- Auto-split on + / 和 / 、 / & ---
	foods := splitFoodNames(foodStr)
	isMulti := len(foods) > 1
	if isMulti && *amount == "各一份" {
		*amount = "1份"
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if isMulti {
		// Multi-food mode: look up each item, show combined results, save as separate entries.
		type foodResult struct {
			food   string
			result *nutrition.NutritionResult
		}
		var results []foodResult
		totalKcal, totalProt, totalFat, totalCarbs := 0.0, 0.0, 0.0, 0.0

		for _, f := range foods {
			fmt.Printf("正在查询 %s%s%s (%s)...\n", ansi(cCyan), f, ansi(cReset), *amount)
			r, err := nutrition.Lookup(f, *brand, *amount, cfg.LLM)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s警告:%s %s 查询失败: %v，跳过\n", ansi(cYellow), ansi(cReset), f, err)
				continue
			}
			results = append(results, foodResult{f, r})
			totalKcal += r.CaloriesKcal
			totalProt += r.ProteinG
			totalFat += r.FatG
			totalCarbs += r.CarbsG
		}

		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "%s错误:%s 所有食物查询均失败\n", ansi(cRed), ansi(cReset))
			os.Exit(1)
		}

		fmt.Printf("\n%s=== 查询结果汇总 ===%s\n", ansi(cCyan), ansi(cReset))
		for i, fr := range results {
			fmt.Printf("  %d. %s%s (%s)%s\n", i+1, ansi(cBold), fr.food, *amount, ansi(cReset))
			fmt.Printf("     热量: %s%.0f kcal%s | 蛋白质: %s%.1fg%s | 脂肪: %s%.1fg%s | 碳水: %s%.1fg%s\n",
				ansi(cYellow), fr.result.CaloriesKcal, ansi(cReset),
				ansi(cCyan), fr.result.ProteinG, ansi(cReset),
				ansi(cYellow), fr.result.FatG, ansi(cReset),
				ansi(cCyan), fr.result.CarbsG, ansi(cReset))
		}
		fmt.Printf("  %s合计:%s 热量: %s%.0f kcal%s | 蛋白质: %s%.1fg%s | 脂肪: %s%.1fg%s | 碳水: %s%.1fg%s\n",
			ansi(cBold), ansi(cReset),
			ansi(cYellow), totalKcal, ansi(cReset),
			ansi(cCyan), totalProt, ansi(cReset),
			ansi(cYellow), totalFat, ansi(cReset),
			ansi(cCyan), totalCarbs, ansi(cReset))

		// Confirm.
		confirmed := *yes || !isTTY(os.Stdin)
		if !confirmed {
			fmt.Println()
			input, err := tui.ReadLine("确认添加全部? [Y/n]: ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
				os.Exit(1)
			}
			input = strings.TrimSpace(strings.ToLower(input))
			confirmed = input == "" || input == "y" || input == "yes"
		}
		if !confirmed {
			fmt.Println("已取消")
			return
		}

		// Save all entries.
		if err := storage.EnsureDataDir(); err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		now := time.Now()
		entries, err := storage.LoadDay(now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}

		for i, fr := range results {
			ts := recordTime.Add(time.Duration(i) * time.Minute)
			entries = append(entries, models.Entry{
				Timestamp:    ts.Format("15:04"),
				Brand:        *brand,
				Food:         fr.food,
				Amount:       *amount,
				CaloriesKcal: fr.result.CaloriesKcal,
				ProteinG:     fr.result.ProteinG,
				FatG:         fr.result.FatG,
				CarbsG:       fr.result.CarbsG,
				Notes:        fr.result.Notes,
			})
		}

		if err := storage.SaveDay(now, entries); err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}

		fmt.Printf("\n%s✓ 已添加 %d 条记录%s\n", ansi(cGreen), len(results), ansi(cReset))
		return
	}

	// --- Single-food mode ---
	displayName := foodStr
	if *brand != "" {
		displayName = *brand + " " + foodStr
	}
	fmt.Printf("正在查询 %s (%s) 的营养信息...\n", displayName, *amount)

	result, err := nutrition.Lookup(foodStr, *brand, *amount, cfg.LLM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	// Display result.
	fmt.Printf("\n%s=== 查询结果 ===%s\n", ansi(cCyan), ansi(cReset))
	fmt.Printf("%s%s%s (%s)\n", ansi(cBold), displayName, ansi(cReset), *amount)
	fmt.Printf("  热量: %s%.0f kcal%s | 蛋白质: %s%.1fg%s | 脂肪: %s%.1fg%s | 碳水: %s%.1fg%s\n",
		ansi(cYellow), result.CaloriesKcal, ansi(cReset),
		ansi(cCyan), result.ProteinG, ansi(cReset),
		ansi(cYellow), result.FatG, ansi(cReset),
		ansi(cCyan), result.CarbsG, ansi(cReset),
	)
	if result.Notes != "" {
		fmt.Printf("  备注: %s%s%s\n", ansi(cDim), result.Notes, ansi(cReset))
	}

	// Confirm (skip with -y or when stdin is not a terminal).
	if !*yes && isTTY(os.Stdin) {
		fmt.Println()
		input, err := tui.ReadLine("确认添加? [Y/n]: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s错误:%s 无法读取输入: %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "" && input != "y" && input != "yes" {
			fmt.Println("已取消")
			return
		}
	}

	// Save.
	if err := storage.EnsureDataDir(); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	now := time.Now()
	entries, err := storage.LoadDay(now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	entry := models.Entry{
		Timestamp:    recordTime.Format("15:04"),
		Brand:        *brand,
		Food:         foodStr,
		Amount:       *amount,
		CaloriesKcal: result.CaloriesKcal,
		ProteinG:     result.ProteinG,
		FatG:         result.FatG,
		CarbsG:       result.CarbsG,
		Notes:        result.Notes,
	}
	entries = append(entries, entry)

	if err := storage.SaveDay(now, entries); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	index := len(entries)
	fmt.Printf("\n%s✓ 已添加 #%d:%s %s [%s]\n",
		ansi(cGreen), index, ansi(cReset), displayName, recordTime.Format("15:04"))
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	dateStr := fs.String("date", "", "日期 (YYYY-MM-DD)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen list [--date YYYY-MM-DD]\n")
	}
	fs.Parse(args)

	var date time.Time
	if *dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", *dateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s 日期格式无效，请使用 YYYY-MM-DD\n", ansi(cRed), ansi(cReset))
			os.Exit(1)
		}
	} else {
		date = time.Now()
	}

	entries, err := storage.LoadDay(date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Printf("%s%s%s 暂无记录\n", ansi(cDim), date.Format("2006-01-02"), ansi(cReset))
		return
	}

	fmt.Printf("\n%s%s%s\n", ansi(cBold), date.Format("2006-01-02"), ansi(cReset))
	printEntryTable(entries, 0)
}

// ---------------------------------------------------------------------------
// edit
// ---------------------------------------------------------------------------

func cmdEdit(args []string) {
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	dateStr := fs.String("date", "", "日期 (YYYY-MM-DD)")
	brand := fs.String("brand", "", "品牌")
	food := fs.String("food", "", "食物名称")
	amount := fs.String("amount", "", "份量")
	calories := fs.Float64("calories", 0, "热量 (kcal)")
	protein := fs.Float64("protein", 0, "蛋白质 (g)")
	fat := fs.Float64("fat", 0, "脂肪 (g)")
	carbs := fs.Float64("carbs", 0, "碳水 (g)")
	notes := fs.String("notes", "", "备注")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen edit <序号> [--date YYYY-MM-DD] [--brand 品牌 --food 名称 --amount 份量 --calories N --protein N --fat N --carbs N --notes 备注]\n")
	}

	pos, flagArgs := splitArgs(args)
	if len(pos) < 1 {
		fs.Usage()
		os.Exit(1)
	}
	fs.Parse(flagArgs)

	index, err := strconv.Atoi(pos[0])
	if err != nil || index < 1 {
		fmt.Fprintf(os.Stderr, "%s错误:%s 序号必须为正整数\n", ansi(cRed), ansi(cReset))
		os.Exit(1)
	}

	// Record which flags were explicitly provided.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if len(setFlags) == 0 {
		fmt.Fprintf(os.Stderr, "%s错误:%s 请至少指定一个要修改的字段\n", ansi(cRed), ansi(cReset))
		os.Exit(1)
	}

	date := parseDate(*dateStr)
	entries, err := storage.LoadDay(date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Printf("%s%s%s 暂无记录\n", ansi(cDim), date.Format("2006-01-02"), ansi(cReset))
		return
	}
	if index > len(entries) {
		fmt.Fprintf(os.Stderr, "%s错误:%s 序号 %d 超出范围 (1-%d)\n", ansi(cRed), ansi(cReset), index, len(entries))
		os.Exit(1)
	}

	entry := &entries[index-1]
	if setFlags["brand"] {
		entry.Brand = *brand
	}
	if setFlags["food"] {
		entry.Food = *food
	}
	if setFlags["amount"] {
		entry.Amount = *amount
	}
	if setFlags["calories"] && *calories >= 0 {
		entry.CaloriesKcal = *calories
	}
	if setFlags["protein"] && *protein >= 0 {
		entry.ProteinG = *protein
	}
	if setFlags["fat"] && *fat >= 0 {
		entry.FatG = *fat
	}
	if setFlags["carbs"] && *carbs >= 0 {
		entry.CarbsG = *carbs
	}
	if setFlags["notes"] {
		entry.Notes = *notes
	}

	if err := storage.SaveDay(date, entries); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	resultName := entry.Food
	if entry.Brand != "" {
		resultName = entry.Brand + " " + entry.Food
	}
	fmt.Printf("%s✓ 已编辑 #%d:%s %s (%s)\n", ansi(cGreen), index, ansi(cReset), resultName, entry.Amount)
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func cmdDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	dateStr := fs.String("date", "", "日期 (YYYY-MM-DD)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen delete <序号> [--date YYYY-MM-DD]\n")
	}

	pos, flagArgs := splitArgs(args)
	if len(pos) < 1 {
		fs.Usage()
		os.Exit(1)
	}
	fs.Parse(flagArgs)

	index, err := strconv.Atoi(pos[0])
	if err != nil || index < 1 {
		fmt.Fprintf(os.Stderr, "%s错误:%s 序号必须为正整数\n", ansi(cRed), ansi(cReset))
		os.Exit(1)
	}

	date := parseDate(*dateStr)

	// Read entry before deleting to display its info.
	entries, err := storage.LoadDay(date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Printf("%s%s%s 暂无记录\n", ansi(cDim), date.Format("2006-01-02"), ansi(cReset))
		return
	}
	if index > len(entries) {
		fmt.Fprintf(os.Stderr, "%s错误:%s 序号 %d 超出范围 (1-%d)\n", ansi(cRed), ansi(cReset), index, len(entries))
		os.Exit(1)
	}

	entry := entries[index-1]

	if err := storage.DeleteEntry(date, index); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	fmt.Printf("%s✓ 已删除 #%d:%s %s (%s)\n", ansi(cGreen), index, ansi(cReset), entry.Food, entry.Amount)
}

// ---------------------------------------------------------------------------
// stats
// ---------------------------------------------------------------------------

func cmdStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dateStr := fs.String("date", "", "日期 (YYYY-MM-DD)")
	week := fs.Bool("week", false, "显示最近 7 天统计")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen stats [--date YYYY-MM-DD] [--week]\n")
	}
	fs.Parse(args)

	if *week {
		fmt.Printf("\n%s最近 7 天统计%s\n\n", ansi(cBold), ansi(cReset))

		now := time.Now()
		totalKcal, totalProt, totalFat, totalCarbs := 0.0, 0.0, 0.0, 0.0

		for i := 6; i >= 0; i-- {
			date := now.AddDate(0, 0, -i)
			entries, err := storage.LoadDay(date)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s警告:%s 无法读取 %s: %v\n", ansi(cYellow), ansi(cReset), date.Format("01-02"), err)
				continue
			}

			kcal, prot, fat, carbs := sumEntries(entries)
			totalKcal += kcal
			totalProt += prot
			totalFat += fat
			totalCarbs += carbs

			label := date.Format("01-02")
			if i == 0 {
				label += " (今天)"
			}
			n := len(entries)
			fmt.Printf("  %s%s%s  %s%2d 条%s  热量: %s%6.0f kcal%s  蛋白质: %s%5.1f g%s  脂肪: %s%5.1f g%s  碳水: %s%5.1f g%s\n",
				ansi(cBold), label, ansi(cReset),
				ansi(cDim), n, ansi(cReset),
				ansi(cYellow), kcal, ansi(cReset),
				ansi(cCyan), prot, ansi(cReset),
				ansi(cYellow), fat, ansi(cReset),
				ansi(cCyan), carbs, ansi(cReset),
			)
		}

		fmt.Printf("%s─────────────────────────────────────────────────────────────────────────%s\n", ansi(cDim), ansi(cReset))
		fmt.Printf("  7 天汇总          热量: %s%6.0f kcal%s  蛋白质: %s%5.1f g%s  脂肪: %s%5.1f g%s  碳水: %s%5.1f g%s\n",
			ansi(cYellow), totalKcal, ansi(cReset),
			ansi(cCyan), totalProt, ansi(cReset),
			ansi(cYellow), totalFat, ansi(cReset),
			ansi(cCyan), totalCarbs, ansi(cReset),
		)
		fmt.Printf("  日均              热量: %s%6.0f kcal%s  蛋白质: %s%5.1f g%s  脂肪: %s%5.1f g%s  碳水: %s%5.1f g%s\n",
			ansi(cYellow), totalKcal/7, ansi(cReset),
			ansi(cCyan), totalProt/7, ansi(cReset),
			ansi(cYellow), totalFat/7, ansi(cReset),
			ansi(cCyan), totalCarbs/7, ansi(cReset),
		)
	} else {
		date := parseDate(*dateStr)
		entries, err := storage.LoadDay(date)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		printSummaryWithTargets(date, entries, cfg.Targets)
	}
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	provider := fs.String("llm-provider", "", "LLM 提供商")
	model := fs.String("llm-model", "", "LLM 模型")
	baseURL := fs.String("base-url", "", "API Base URL")
	apiKey := fs.String("api-key", "", "API Key (支持 env:VAR 格式)")
	caloriesGoal := fs.Float64("calories-goal", 0, "每日热量目标 (kcal)")
	proteinGoal := fs.Float64("protein-goal", 0, "每日蛋白质目标 (g)")
	xiaomiUserID := fs.String("xiaomi-user-id", "", "小米账号 (邮箱或手机号)")
	xiaomiPwd := fs.String("xiaomi-password", "", "小米密码 (支持 env:VAR 格式)")
	height := fs.Float64("height", 0, "身高 (cm)")
	birthDate := fs.String("birth-date", "", "出生日期 (YYYY-MM-DD)")
	gender := fs.String("gender", "", "性别 (male/female)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen config [--llm-provider P --llm-model M --base-url URL --api-key KEY --calories-goal N --protein-goal N]\n")
	}
	fs.Parse(args)

	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	// Save if any flags were provided.
	if len(setFlags) > 0 {
		if setFlags["llm-provider"] {
			cfg.LLM.Provider = *provider
		}
		if setFlags["llm-model"] {
			cfg.LLM.Model = *model
		}
		if setFlags["base-url"] {
			cfg.LLM.BaseURL = *baseURL
		}
		if setFlags["api-key"] {
			cfg.LLM.APIKey = *apiKey
		}
		if setFlags["calories-goal"] && *caloriesGoal >= 0 {
			cfg.Targets.CaloriesGoal = *caloriesGoal
		}
		if setFlags["protein-goal"] && *proteinGoal >= 0 {
			cfg.Targets.ProteinGoal = *proteinGoal
		}
		if setFlags["xiaomi-user-id"] {
			cfg.Body.Scale.XiaomiUserID = *xiaomiUserID
		}
		if setFlags["xiaomi-password"] {
			cfg.Body.Scale.XiaomiPassword = *xiaomiPwd
		}
		if setFlags["height"] && *height > 0 {
			cfg.Body.HeightCm = *height
		}
		if setFlags["birth-date"] {
			cfg.Body.BirthDate = *birthDate
		}
		if setFlags["gender"] {
			cfg.Body.Gender = *gender
		}
		// Enable Xiaomi provider when credentials are set
		if setFlags["xiaomi-user-id"] || setFlags["xiaomi-password"] {
			cfg.Body.Scale.Provider = "xiaomi"
		}

		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		fmt.Printf("%s✓ 配置已保存%s\n", ansi(cGreen), ansi(cReset))
	}

	// Display current config.
	fmt.Printf("\n%sLLM 配置:%s\n", ansi(cBold), ansi(cReset))
	fmt.Printf("  提供商:   %s\n", cfg.LLM.Provider)
	fmt.Printf("  模型:     %s\n", cfg.LLM.Model)
	fmt.Printf("  Base URL: %s\n", cfg.LLM.BaseURL)
	fmt.Printf("  API Key:  %s\n", maskAPIKey(cfg.LLM.APIKey))
	fmt.Printf("\n%s目标配置:%s\n", ansi(cBold), ansi(cReset))
	fmt.Printf("  每日热量:   %.0f kcal\n", cfg.Targets.CaloriesGoal)
	fmt.Printf("  每日蛋白质: %.0f g\n", cfg.Targets.ProteinGoal)

	fmt.Printf("\n%s身体指标配置:%s\n", ansi(cBold), ansi(cReset))
	heightDisplay := fmt.Sprintf("%.0f", cfg.Body.HeightCm)
	if cfg.Body.HeightCm <= 0 {
		heightDisplay = "(未设置)"
	}
	fmt.Printf("  身高:     %s cm\n", heightDisplay)
	bd := cfg.Body.BirthDate
	if bd == "" {
		bd = "(未设置)"
	}
	fmt.Printf("  出生日期: %s\n", bd)
	fmt.Printf("  性别:     %s\n", cfg.Body.Gender)

	fmt.Printf("\n%s智能秤配置:%s\n", ansi(cBold), ansi(cReset))
	if cfg.Body.Scale.Provider == "xiaomi" {
		fmt.Printf("  提供商:   小米 (已配置)\n")
		fmt.Printf("  账号:     %s\n", maskMiddle(cfg.Body.Scale.XiaomiUserID))
		fmt.Printf("  密码:     %s\n", maskAPIKey(cfg.Body.Scale.XiaomiPassword))
	} else {
		fmt.Printf("  提供商:   (未配置)\n")
		fmt.Printf("  提示: essen config --xiaomi-user-id USER --xiaomi-password PASS\n")
	}
}

// ---------------------------------------------------------------------------
// weight
// ---------------------------------------------------------------------------

func cmdWeight(args []string) {
	fs := flag.NewFlagSet("weight", flag.ExitOnError)
	list := fs.Bool("list", false, "列出所有体测记录")
	xiaomi := fs.Bool("xiaomi", false, "从小米云导入体测数据")
	wcfg := fs.Bool("config", false, "设置小米账号(同 essen config --xiaomi-*)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: essen weight [kg] [--list] [--xiaomi] [--config]\n")
	}

	pos, flagArgs := splitArgs(args)
	fs.Parse(flagArgs)

	if *wcfg {
		cmdConfig([]string{"--help"})
		return
	}

	if *xiaomi {
		cmdWeightXiaomi()
		return
	}

	if len(pos) > 0 {
		weight, err := strconv.ParseFloat(pos[0], 64)
		if err != nil || weight <= 0 || weight > 500 {
			fmt.Fprintf(os.Stderr, "%s错误:%s 无效体重值: %s (请输入数字，如 72.5)\n", ansi(cRed), ansi(cReset), pos[0])
			os.Exit(1)
		}
		if err := body.AddManual(weight); err != nil {
			fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
			os.Exit(1)
		}
		fmt.Printf("%s✓%s 已记录体重: %.1f kg\n", ansi(cGreen), ansi(cReset), weight)
		return
	}

	if *list || (!*xiaomi && !*wcfg && len(pos) == 0) {
		cmdWeightList()
		return
	}
}

func cmdWeightXiaomi() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	userID := config.ResolveAPIKey(cfg.Body.Scale.XiaomiUserID)
	password := config.ResolveAPIKey(cfg.Body.Scale.XiaomiPassword)

	if cfg.Body.Scale.Provider != "xiaomi" || userID == "" || password == "" {
		fmt.Fprintf(os.Stderr, "%s小米账号未配置。请先运行:%s\n", ansi(cYellow), ansi(cReset))
		fmt.Fprintf(os.Stderr, "  essen config --xiaomi-user-id YOUR_ACCOUNT --xiaomi-password env:XIAOMI_PASS\n")
		os.Exit(1)
	}

	fmt.Printf("正在从小米云同步体测数据...\n")
	fetched, err := body.FetchXiaomi(userID, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if len(fetched) == 0 {
		fmt.Println("未找到体测记录")
		return
	}

	existing, err := body.LoadMeasurements()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	merged := body.MergeMeasurements(existing, fetched)
	if err := body.SaveMeasurements(merged); err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	newCount := len(merged) - len(existing)
	fmt.Printf("%s✓%s 已从小米云导入 %d 条记录 (新增 %d 条)\n",
		ansi(cGreen), ansi(cReset), len(fetched), newCount)

	latest := merged[len(merged)-1]
	fmt.Printf("\n最新: %.1f kg", latest.WeightKg)
	if latest.BodyFatPct > 0 {
		fmt.Printf(" | 体脂: %.1f%%", latest.BodyFatPct)
	}
	if latest.MuscleMassKg > 0 {
		fmt.Printf(" | 肌肉: %.1f kg", latest.MuscleMassKg)
	}
	fmt.Println()
}

func cmdWeightList() {
	measurements, trend, err := body.ListMeasurements()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	if len(measurements) == 0 {
		fmt.Printf("%s暂无体测记录。%s\n", ansi(cDim), ansi(cReset))
		fmt.Println("用法: essen weight 72.5  (手动记录体重)")
		fmt.Println("      essen weight --xiaomi (从小米云导入)")
		return
	}

	fmt.Printf("\n%s📊 体测记录 (%d 条)%s\n", ansi(cBold), trend.Count, ansi(cReset))
	fmt.Printf("  %s范围: %s ~ %s%s\n", ansi(cDim), trend.FirstDate, trend.LastDate, ansi(cReset))
	fmt.Println()

	fmt.Printf("  %s%-12s %8s %8s %8s %8s %6s%s\n",
		ansi(cBold), "日期", "体重", "体脂", "肌肉", "骨骼", "来源", ansi(cReset))

	show := measurements
	if len(show) > 14 {
		show = show[len(show)-14:]
	}

	for _, m := range show {
		fatStr := "-"
		if m.BodyFatPct > 0 {
			fatStr = fmt.Sprintf("%.1f%%", m.BodyFatPct)
		}
		muscleStr := "-"
		if m.MuscleMassKg > 0 {
			muscleStr = fmt.Sprintf("%.1f", m.MuscleMassKg)
		}
		boneStr := "-"
		if m.BoneMassKg > 0 {
			boneStr = fmt.Sprintf("%.1f", m.BoneMassKg)
		}
		fmt.Printf("  %-12s %8.1f %8s %8s %8s %6s\n",
			m.Date, m.WeightKg, fatStr, muscleStr, boneStr, body.FormatSource(m.Source))
	}

	if trend.Count > 0 {
		fmt.Printf("\n  %s趋势:%s\n", ansi(cBold), ansi(cReset))
		fmt.Printf("    最新:     %.1f kg\n", trend.Latest.WeightKg)
		fmt.Printf("    平均:     %.1f kg\n", trend.WeightAvg)
		fmt.Printf("    范围:     %.1f ~ %.1f kg\n", trend.WeightMin, trend.WeightMax)
		if trend.Delta7Day != 0 {
			fmt.Printf("    7天变化:  %s\n", body.FormatDelta(trend.Delta7Day))
		}
		if trend.Delta30Day != 0 {
			fmt.Printf("    30天变化: %s\n", body.FormatDelta(trend.Delta30Day))
		}
	}
	fmt.Println()
}

// maskMiddle masks the middle part of a string for display.
func maskMiddle(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	runes := []rune(s)
	show := 2
	if len(runes) < 6 {
		show = 1
	}
	return string(runes[:show]) + strings.Repeat("*", len(runes)-2*show) + string(runes[len(runes)-show:])
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sumEntries returns the total macros across a slice of entries.
func sumEntries(entries []models.Entry) (kcal, prot, fat, carbs float64) {
	for _, e := range entries {
		kcal += e.CaloriesKcal
		prot += e.ProteinG
		fat += e.FatG
		carbs += e.CarbsG
	}
	return
}

// printEntryTable prints entries in a formatted table with a summary row.
func printEntryTable(entries []models.Entry, startIdx int) {
	if len(entries) == 0 {
		return
	}

	fmt.Println()
	// Header.
	fmt.Printf("  %s%-3s  %-6s  %-8s  %-16s  %-10s  %7s  %7s  %7s  %7s%s\n",
		ansi(cBold),
		"#", "时间", "品牌", "食物", "份量", "kcal", "蛋白", "脂肪", "碳水",
		ansi(cReset),
	)

	for i, e := range entries {
		num := startIdx + i + 1
		brand := e.Brand
		if brand == "" {
			brand = "—"
		} else {
			brand = truncateRunes(brand, 8)
		}
		food := truncateRunes(e.Food, 16)
		amount := truncateRunes(e.Amount, 10)
		fmt.Printf("  %-3d  %-6s  %-8s  %-16s  %-10s  %7.0f  %7.1f  %7.1f  %7.1f\n",
			num, e.Timestamp, brand, food, amount,
			e.CaloriesKcal, e.ProteinG, e.FatG, e.CarbsG,
		)
	}

	// Separator & summary.
	kcal, prot, fat, carbs := sumEntries(entries)
	fmt.Printf("  %s───  ──────  ────────  ────────────────  ──────────  ───────  ───────  ───────  ───────%s\n",
		ansi(cDim), ansi(cReset))
	fmt.Printf("  %s汇总                                                                      %s%7.0f%s  %s%7.1f%s  %s%7.1f%s  %s%7.1f%s\n",
		ansi(cBold),
		ansi(cYellow), kcal, ansi(cReset),
		ansi(cCyan), prot, ansi(cReset),
		ansi(cYellow), fat, ansi(cReset),
		ansi(cCyan), carbs, ansi(cReset),
	)
	fmt.Println()
}

// printSummaryWithTargets prints a single‑day summary block with goal progress.
func printSummaryWithTargets(date time.Time, entries []models.Entry, targets config.Targets) {
	kcal, prot, fat, carbs := sumEntries(entries)
	fmt.Printf("\n%s%s%s  %d 条记录\n\n", ansi(cBold), date.Format("2006-01-02"), ansi(cReset), len(entries))

	if targets.CaloriesGoal > 0 || targets.ProteinGoal > 0 {
		fmt.Printf("  %s🎯 今日进度%s\n", ansi(cCyan), ansi(cReset))
	}

	if targets.CaloriesGoal > 0 {
		pct := kcal / targets.CaloriesGoal * 100
		if pct > 100 {
			pct = 100
		}
		bar := progressBar(pct)
		fmt.Printf("  热量:   %s%6.0f%s / %.0f kcal  %s %3.0f%%\n",
			ansi(cYellow), kcal, ansi(cReset), targets.CaloriesGoal, bar, pct)
	} else {
		fmt.Printf("  热量:   %s%.0f kcal%s\n", ansi(cYellow), kcal, ansi(cReset))
	}

	if targets.ProteinGoal > 0 {
		pct := prot / targets.ProteinGoal * 100
		if pct > 100 {
			pct = 100
		}
		bar := progressBar(pct)
		fmt.Printf("  蛋白质: %s%5.1f%s / %.1f g    %s %3.0f%%\n",
			ansi(cCyan), prot, ansi(cReset), targets.ProteinGoal, bar, pct)
	} else {
		fmt.Printf("  蛋白质: %s%.1f g%s\n", ansi(cCyan), prot, ansi(cReset))
	}

	fmt.Printf("\n  脂肪:   %s%.1f g%s\n", ansi(cYellow), fat, ansi(cReset))
	fmt.Printf("  碳水:   %s%.1f g%s\n", ansi(cCyan), carbs, ansi(cReset))

	if targets.CaloriesGoal > 0 || targets.ProteinGoal > 0 {
		remainingKcal := targets.CaloriesGoal - kcal
		remainingProt := targets.ProteinGoal - prot
		fmt.Printf("\n  %s剩余需要:%s", ansi(cDim), ansi(cReset))
		if targets.CaloriesGoal > 0 && remainingKcal > 0 {
			fmt.Printf(" %.0f kcal", remainingKcal)
		}
		if targets.ProteinGoal > 0 && remainingProt > 0 {
			fmt.Printf(" / %.1f g 蛋白质", remainingProt)
		}
		if remainingKcal <= 0 && remainingProt <= 0 {
			fmt.Printf(" %s✓ 已达标!%s", ansi(cGreen), ansi(cReset))
		}
		fmt.Println()
	}
	fmt.Println()
}

// progressBar returns a 10‑block ASCII progress bar string with color.
func progressBar(pct float64) string {
	filled := int(pct / 10.0)
	if filled < 0 {
		filled = 0
	}
	if filled > 10 {
		filled = 10
	}
	empty := 10 - filled

	var barColor string
	if pct >= 80 {
		barColor = cGreen
	} else if pct >= 40 {
		barColor = cYellow
	} else {
		barColor = cRed
	}

	return fmt.Sprintf("%s%s%s%s",
		ansi(barColor),
		strings.Repeat("█", filled),
		ansi(cDim)+strings.Repeat("░", empty),
		ansi(cReset),
	)
}

// cmdToday combines list and stats output for today.
func cmdToday() {
	date := time.Now()
	entries, err := storage.LoadDay(date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s %v\n", ansi(cRed), ansi(cReset), err)
		os.Exit(1)
	}

	fmt.Printf("\n%s%s%s\n", ansi(cBold), date.Format("2006-01-02"), ansi(cReset))

	if len(entries) == 0 {
		fmt.Printf("%s暂无记录%s\n", ansi(cDim), ansi(cReset))
	} else {
		printEntryTable(entries, 0)
	}

	printSummaryWithTargets(date, entries, cfg.Targets)
}

// splitFoodNames splits a food string on common conjunction markers.
// Examples: "A+B" → ["A","B"], "A和B" → ["A","B"], "A、B" → ["A","B"]
func splitFoodNames(s string) []string {
	for _, sep := range []string{"+", "和", "、", "&", "/", ","} {
		parts := strings.Split(s, sep)
		if len(parts) >= 2 {
			var result []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					result = append(result, p)
				}
			}
			if len(result) >= 2 {
				return result
			}
		}
	}
	return []string{s}
}

// truncateRunes truncates a string to max runes, appending ".." if shortened.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 2 {
		return string(runes[:max])
	}
	return string(runes[:max-2]) + ".."
}

// parseDate parses a date string in YYYY-MM-DD format.
// Returns today when dateStr is empty.
func parseDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Now()
	}
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s错误:%s 日期格式无效，请使用 YYYY-MM-DD\n", ansi(cRed), ansi(cReset))
		os.Exit(1)
	}
	return d
}

// maskAPIKey returns a display‑safe version of an API key.
func maskAPIKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	// For env: references, show them as‑is.
	if strings.HasPrefix(key, "env:") {
		return key
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:7] + "..." + key[len(key)-4:]
}
