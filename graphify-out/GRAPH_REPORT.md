# Graph Report - .  (2026-07-29)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 170 nodes · 375 edges · 9 communities (8 shown, 1 thin omitted)
- Extraction: 89% EXTRACTED · 11% INFERRED · 0% AMBIGUOUS · INFERRED: 41 edges (avg confidence: 0.8)
- Token cost: 468 input · 612 output

## Graph Freshness
- Built from commit: `3ec67fe4`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Day Data Storage
- Terminal Input Handling
- FatSecret Lookup
- Essen Config & Storage
- Body Measurement Trends
- Xiaomi Scale API
- OpenFoodFacts Lookup
- Configuration Management
- Essen CLI

## God Nodes (most connected - your core abstractions)
1. `ansi()` - 16 edges
2. `Essen` - 13 edges
3. `NutritionResult` - 12 edges
4. `cmdAdd()` - 12 edges
5. `LoadDay()` - 11 edges
6. `main()` - 11 edges
7. `ReadLine()` - 10 edges
8. `Load()` - 9 edges
9. `BodyMeasurement` - 9 edges
10. `Lookup()` - 9 edges

## Surprising Connections (you probably didn't know these)
- `cmdWeight()` --calls--> `AddManual()`  [INFERRED]
  main.go → internal/body/body.go
- `cmdWeightList()` --calls--> `FormatSource()`  [INFERRED]
  main.go → internal/body/body.go
- `cmdWeightList()` --calls--> `FormatDelta()`  [INFERRED]
  main.go → internal/body/body.go
- `cmdWeightXiaomi()` --calls--> `FetchXiaomi()`  [INFERRED]
  main.go → internal/body/xiaomi.go
- `cmdAdd()` --calls--> `Load()`  [INFERRED]
  main.go → internal/config/config.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Nutrition Data Fallback Chain** — readme_fatsecret_cn, readme_openfoodfacts, readme_llm_estimation [EXTRACTED 1.00]

## Communities (9 total, 1 thin omitted)

### Community 0 - "Day Data Storage"
Cohesion: 0.17
Nodes (33): DataDir(), DayPath(), DeleteEntry(), EnsureDataDir(), Time, LoadDay(), SaveDay(), ansi() (+25 more)

### Community 1 - "Terminal Input Handling"
Cohesion: 0.16
Nodes (24): decodeUTF8ByteByByte(), displayWidth(), displayWidthRunes(), getTermios(), File, isTTY(), makeRaw(), readEscSeq() (+16 more)

### Community 2 - "FatSecret Lookup"
Cohesion: 0.17
Nodes (22): brandMatches(), extractFromFatsecretHTML(), fatsecretLookup(), foodName(), parseFatsecretRow(), searchFatsecret(), contains(), T (+14 more)

### Community 3 - "Essen Config & Storage"
Cohesion: 0.11
Nodes (19): essen add, API Key, Calories Goal, essen config, Config File Path (~/.config/essen/config.json), Daily JSON Storage, Daily Storage Path (~/.local/share/essen/YYYY-MM-DD.json), Essen (+11 more)

### Community 4 - "Body Measurement Trends"
Cohesion: 0.21
Nodes (16): TrendResult, AddManual(), ComputeTrend(), DataDir(), DataPath(), ensureDir(), FormatDelta(), FormatSource() (+8 more)

### Community 5 - "Xiaomi Scale API"
Cohesion: 0.19
Nodes (17): xiaomiAPIResponse, xiaomiScaleRecord, xiaomiSession, Client, decryptRC4(), encryptRC4(), fetchScaleData(), FetchXiaomi() (+9 more)

### Community 6 - "OpenFoodFacts Lookup"
Cohesion: 0.32
Nodes (11): buildQuery(), editDistance(), min3(), nameMatches(), openFoodFactsLookup(), productToResult(), servingQuantityFloat(), toFloat() (+3 more)

### Community 7 - "Configuration Management"
Cohesion: 0.42
Nodes (9): BodyConfig, Config, LLMConfig, ScaleConfig, Targets, ConfigPath(), DefaultConfig(), Load() (+1 more)

## Knowledge Gaps
- **12 isolated node(s):** `essen`, `chatResponse`, `Interactive Add`, `Smart Splitting`, `Goal Progress Bars` (+7 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `cmdAdd()` connect `Day Data Storage` to `Terminal Input Handling`, `FatSecret Lookup`, `Configuration Management`?**
  _High betweenness centrality (0.368) - this node is a cross-community bridge._
- **Why does `cmdWeightXiaomi()` connect `Body Measurement Trends` to `Day Data Storage`, `Xiaomi Scale API`, `Configuration Management`?**
  _High betweenness centrality (0.232) - this node is a cross-community bridge._
- **Why does `ReadLine()` connect `Terminal Input Handling` to `Day Data Storage`?**
  _High betweenness centrality (0.217) - this node is a cross-community bridge._
- **Are the 6 inferred relationships involving `cmdAdd()` (e.g. with `Load()` and `Lookup()`) actually correct?**
  _`cmdAdd()` has 6 INFERRED edges - model-reasoned connections that need verification._
- **Are the 6 inferred relationships involving `LoadDay()` (e.g. with `cmdAdd()` and `cmdDelete()`) actually correct?**
  _`LoadDay()` has 6 INFERRED edges - model-reasoned connections that need verification._
- **What connects `essen`, `chatResponse`, `Interactive Add` to the rest of the system?**
  _12 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Essen Config & Storage` be split into smaller, more focused modules?**
  _Cohesion score 0.11428571428571428 - nodes in this community are weakly interconnected._