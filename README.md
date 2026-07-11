# Essen — CLI Food Diary for Fitness

> **Track what you eat, hit your macros, build your body.**

Essen is a Go CLI tool for people who mean it — lifters, runners, and anyone
counting calories and protein to fuel real progress. Log food in seconds, get
real nutritional data from public sources, and watch your daily goals fill up.

```
$ essen today

╔══════════════════════════════════════╗
║  2026-07-11                    3 条  ║
╚══════════════════════════════════════╝

  🎯 今日进度
  热量:   1580 / 2500 kcal  ███████░░░ 63%
  蛋白质: 78.0 / 120.0 g    ██████░░░░ 60%

  剩余需要: 920 kcal / 42.0 g 蛋白质
```

---

## Features

### 🖊️ Interactive Add

Run `essen add` with no arguments and enter details step by step — brand, food,
amount, time. Arrow keys to edit, Tab/Enter to confirm. No syntax to remember.

```
$ essen add
  品牌 (可选): 711
  食物名称: 鸡胸肉饭团
  份量: 1份
  时间 [HH:MM] (回车=现在 12:30): 12:00
```

### 🔗 Smart Splitting

Enter multiple foods at once — Essen splits them automatically on `+`, `和`,
`、`, `&`, `/`, or `,`. Each item is looked up independently.

```
$ essen add "鸡胸肉+西兰花" --amount "200g"
  正在查询 鸡胸肉 (200g)...
  正在查询 西兰花 (200g)...

  === 查询结果汇总 ===
  1. 鸡胸肉 (200g)
     热量: 330 kcal | 蛋白质: 62.0g | 脂肪: 7.2g | 碳水: 0.0g
  2. 西兰花 (200g)
     热量: 68 kcal  | 蛋白质: 5.6g  | 脂肪: 0.8g | 碳水: 13.2g

  合计: 热量: 398 kcal | 蛋白质: 67.6g | 脂肪: 8.0g | 碳水: 13.2g
```

### 🥩 Real Nutrition Data

Essen queries **actual product data** instead of guessing. The fallback chain:

1. **FatSecret CN** — scraped serving-level data for Chinese products
2. **OpenFoodFacts** — global open food database (per 100g, auto-scaled)
3. **LLM estimation** — as a fallback when no public data exists

You get real numbers from real products — not GPT hallucinations.

### 📊 Goal Progress Bars

Daily targets for calories and protein, displayed as ASCII progress bars
in `essen today` and `essen stats`. Colored by threshold:
- 🔴 <40% — falling behind
- 🟡 40–80% — on track
- 🟢 ≥80% — almost there

Customise your targets:

```bash
essen config --calories-goal 2500 --protein-goal 120
```

### 🧩 Zero External Dependencies

Essen is built entirely on the Go standard library. No npm, no pip, no runtime
bloat. A single static binary, ready to run on any Linux machine.

- Terminal raw mode via `syscall` — no ncurses
- JSON config and storage — no database
- HTTP client via `net/http` — no curl wrappers

### 💾 Daily JSON Storage

Every day is a plain JSON file. Portable, backup-friendly, grep-able.

```
~/.local/share/essen/
├── 2026-07-10.json
├── 2026-07-11.json
└── ...
```

```json
[
  {
    "timestamp": "08:00",
    "brand": "蒙牛",
    "food": "纯牛奶",
    "amount": "250ml",
    "calories_kcal": 162.5,
    "protein_g": 7.5,
    "fat_g": 8.8,
    "carbs_g": 12.0,
    "notes": "fatsecret.cn | 纯牛奶 (蒙牛)"
  }
]
```

---

## Quick Start

### Install

```bash
# Requires Go 1.21+
cd Essen
go build -o essen .
sudo mv essen /usr/local/bin/
```

### First Run

```bash
# Set up your LLM API key (needed as fallback for unknown foods)
essen config --api-key env:OPENAI_API_KEY

# (Optional) Set daily targets for fitness tracking
essen config --calories-goal 2500 --protein-goal 120
```

### Log Your First Meal

```bash
# Quick add — look up and save in one shot
essen add 鸡胸肉 --amount "200g"

# Interactive mode — step-by-step prompts
essen add

# Multi-food — auto-split on +
essen add "鸡胸肉+西兰花" --amount "200g"

# With brand for more accurate results
essen add "饭团" --brand "711" --amount "1份" --time "12:00"
```

### Check Your Progress

```bash
# Today's entries + goal progress bars
essen today

# Summary only
essen stats

# Weekly overview
essen stats --week

# Browse a specific date
essen list --date 2026-07-10
```

### Edit or Delete

```bash
# Edit fields on entry #1
essen edit 1 --calories 300 --protein 20

# Delete entry #2
essen delete 2
```

---

## Configuration

Essen reads from `~/.config/essen/config.json`. Manage it through the CLI:

```bash
essen config                              # view current settings
essen config --api-key env:OPENAI_API_KEY # set API key
essen config --llm-provider deepseek      # change provider
essen config --llm-model deepseek-chat    # change model
essen config --base-url https://api.deepseek.com/v1
essen config --calories-goal 2500         # daily target
essen config --protein-goal 120           # daily target
```

**API Key** supports `env:VAR_NAME` syntax — the actual key stays in your
environment. Never hardcoded.

Default config applies sensible fitness targets (2500 kcal / 120 g protein)
and uses `gpt-4o-mini` via OpenAI.

---

## Command Reference

| Command | Description |
|---------|-------------|
| `essen add [food] [flags]` | Add a food entry. Interactive if no food given. |
| `essen add "A+B"` | Split on `+`, `和`, `、`, `&`, `/`, `,` — one entry per food |
| `essen today` | Show today's entries + goal progress bars |
| `essen list [--date]` | Tabular listing of entries for a day |
| `essen edit <n> [flags]` | Edit fields on entry #n |
| `essen delete <n> [--date]` | Delete entry #n |
| `essen stats [--date] [--week]` | Daily or 7-day nutrition summary |
| `essen config [flags]` | View or update configuration |

**Add flags:** `--brand`, `--food`, `--amount`, `--time HH:MM`, `-y` (skip prompt)

**Edit flags:** `--brand`, `--food`, `--amount`, `--calories`, `--protein`,
`--fat`, `--carbs`, `--notes`, `--date`

---

## Data Storage

| Path | Contents |
|------|----------|
| `~/.local/share/essen/YYYY-MM-DD.json` | Daily food entries (JSON array) |
| `~/.config/essen/config.json` | LLM config + daily targets |

Both are plain text. Version-controlled, backed up, synced — however you like.

---

## Why "Essen"?

German for **eating**. Short, memorable, and exactly what it does.

---

## License

MIT
