# Essen — 饮食记录 CLI

> **记录每一口，吃出更好体型。**

Essen 是一个面向健身/增肌人群的终端饮食记录工具。用 Go 编写，零外部依赖，
纯文本存储。输入食物名即可自动查询真实营养数据，每日热量和蛋白质进度一目了然。

---

## 功能

### 🖊️ 交互式添加

直接运行 `essen add`，逐行输入品牌、食物、份量、时间。支持方向键编辑、
Home/End 跳转、退格/删除。无需记忆任何命令行语法。

### 🔗 智能拆分

输入 "鸡胸肉+西兰花"，Essen 会自动按 `+` / `和` / `、` / `&` / `/` / `,`
拆分为两条独立记录，分别查询营养数据并汇总展示。

### 🥩 真实营养数据

按优先级三级查询，尽量返回真实产品数据而非 AI 猜测：

1. **FatSecret CN** — 爬取 fatsecret.cn 的中文产品数据（份量级，无需 API Key）
2. **OpenFoodFacts** — 全球开源食品数据库（按 100g 标准，自动按份量缩放）
3. **LLM 估算** — 以上均无结果时，由 LLM 参考标准营养数据估算

### 📊 目标进度条

`essen today` / `essen stats` 展示每日热量和蛋白质的 ASCII 进度条。
颜色阈值：<40% 红色 · 40–80% 黄色 · ≥80% 绿色。

可自定义每日目标：

```bash
essen config --calories-goal 2500 --protein-goal 120
```

### 🧩 零外部依赖

仅使用 Go 标准库实现。终端原生模式通过 `syscall` 直接操作 termios，
无需 ncurses 或任何第三方库。编译为单文件静态二进制。

### 💾 每日 JSON 存储

每天一个纯 JSON 文件，`~/.local/share/essen/YYYY-MM-DD.json`。
纯文本、可备份、可 grep、可纳入版本管理。

---

## 快速开始

```bash
# 编译安装（需 Go 1.21+）
cd Essen
go build -o essen .
sudo mv essen /usr/local/bin/

# 配置 LLM API Key（未知食物查询的备用方案）
essen config --api-key env:OPENAI_API_KEY

# 设置每日目标
essen config --calories-goal 2500 --protein-goal 120

# 添加记录
essen add 鸡胸肉 --amount "200g"
essen add "鸡胸肉+西兰花" --amount "200g"
essen add --brand "711" --food "饭团" --amount "1份"

# 互动式添加
essen add

# 查看今日进度
essen today

# 统计
essen stats
essen stats --week
```

---

## 命令列表

| 命令 | 说明 |
|------|------|
| `essen add [食物] [参数]` | 添加记录。不带食物名称则进入互动模式 |
| `essen add "A+B"` | 自动拆分（支持 +、和、、、&、/、,） |
| `essen today` | 今日记录 + 目标进度条 |
| `essen list [--date]` | 表格形式列出某日记录 |
| `essen edit <序号> [参数]` | 编辑指定条目 |
| `essen delete <序号> [--date]` | 删除指定条目 |
| `essen stats [--date] [--week]` | 日/周营养汇总 |
| `essen config [参数]` | 查看/修改配置 |

**添加参数：** `--brand`、`--food`、`--amount`、`--time HH:MM`、`-y`（跳过确认）

**编辑参数：** `--brand`、`--food`、`--amount`、`--calories`、`--protein`、
`--fat`、`--carbs`、`--notes`、`--date`

---

## 数据存储

| 路径 | 内容 |
|------|------|
| `~/.local/share/essen/YYYY-MM-DD.json` | 每日饮食记录（JSON 数组） |
| `~/.config/essen/config.json` | LLM 配置 + 每日目标 |

均为纯文本文件，备份迁移方便。

---

## 为什么叫 Essen？

德语 "吃"。短、准、就是它该做的事。

---

## 许可证

MIT
