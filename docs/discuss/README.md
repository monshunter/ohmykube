# 讨论存档规范

## 1 目录结构

```
docs/discuss/
├── README.md                   # 本文件，规范说明
├── INDEX.md                    # 文档索引
├── ${subject}-by-${agent}.md   # Agent 分析文档
└── ${subject}-summary.md       # 汇总文档
```

## 2 Agent 名称标准化

| Agent | 标准名称 |
|-------|----------|
| Claude | `claude` |
| Codex | `codex` |
| Gemini | `gemini` |

## 3 命名规范

| 类型 | 命名模式 | 示例 |
|------|----------|------|
| Agent 分析 | `${subject}-by-${agent}.md` | `cache-strategy-by-claude.md` |
| 汇总文档 | `${subject}-summary.md` | `cache-strategy-summary.md` |

## 4 文档模板

### 4.1 Agent 分析模板

```markdown
# ${Subject} 分析

> **Agent**: Claude
> **日期**: YYYY-MM-DD

## 1 问题理解

对问题的理解和分析背景。

## 2 分析过程

### 2.1 方案一

描述和评估。

### 2.2 方案二

描述和评估。

## 3 建议方案

推荐的方案及理由。

## 4 风险与注意事项

- 风险点 1
- 注意事项 1
```

### 4.2 汇总文档模板

```markdown
# ${Subject} 讨论汇总

> **日期**: YYYY-MM-DD

## 1 背景

描述讨论的背景和目标。

## 2 各方观点

### 2.1 Claude 观点

来源：[详细分析](./xxx-by-claude.md)

- 要点 1
- 要点 2

### 2.2 Gemini 观点

来源：[详细分析](./xxx-by-gemini.md)

- 要点 1
- 要点 2

## 3 决策

**采纳方案**：简要描述最终方案

**决策依据**：
- 原因 1
- 原因 2

**来源**：主要采纳自 [Agent 名称] 的建议
```

## 5 汇总规则

汇总各 Agent 建议时，必须遵守以下规则：

1. **标注来源**：每个决策点必须标注来源 Agent
2. **保留链接**：汇总文档必须链接到各 Agent 的原始分析
3. **明确决策**：必须明确最终采纳的方案

## 6 检查清单

完成讨论文档前，确认以下事项：

- [ ] Agent 名称使用标准化格式
- [ ] 汇总文档标注了决策来源
- [ ] 汇总文档链接了各 Agent 分析
- [ ] 已更新 `INDEX.md` 索引
