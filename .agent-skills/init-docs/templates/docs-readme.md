# 项目文档

## 1 文档导航

| 目录 | 用途 | 入口 |
|------|------|------|
| [设计文档](./spec/) | 架构设计、模块规格 | [INDEX](./spec/INDEX.md) |
| [计划文档](./plan/) | 实施计划、Checklist | [INDEX](./plan/INDEX.md) |
| [审查报告](./reports/) | Review、评估报告 | [INDEX](./reports/INDEX.md) |
| [讨论存档](./discuss/) | Agent 分析讨论 | [INDEX](./discuss/INDEX.md) |
| [API 定义](./apis/) | 接口定义（JSON） | [INDEX](./apis/INDEX.md) |
| [工作日志](./work-journal/) | 开发日志 | [INDEX](./work-journal/INDEX.md) |
| [Bug 知识库](./bugs/) | Bug 诊断记录与模式库 | [INDEX](./bugs/INDEX.md) |

## 2 文档规范速查

### 2.1 元信息格式

设计文档和计划文档必须在头部包含元信息：

```markdown
> **版本**: X.Y
> **状态**: draft | active | superseded | deprecated
> **更新日期**: YYYY-MM-DD
```

**状态值说明**：

| 状态 | 含义 |
|------|------|
| `draft` | 草稿，尚未正式生效 |
| `active` | 生效中，当前有效版本 |
| `superseded` | 已被取代，需注明新文档路径 |
| `deprecated` | 已废弃，不再适用 |

### 2.2 Markdown 层级规范

```markdown
# 文档标题      （仅用于主标题，每文档一个）
## 1 一级章节   （数字编号）
### 1.1 二级章节（层级编号）
#### 1.1.1 三级章节（层级编号）
```

### 2.3 目录规范文件

每个目录包含以下规范文件：

| 文件 | 用途 |
|------|------|
| `README.md` | 该目录的规范说明（命名、模板、检查清单） |
| `INDEX.md` | 文档索引导航 |

## 3 关联文档

- [CLAUDE.md](../CLAUDE.md) — Agent 编码指令
- [工作日志规范](./work-journal/README.md) — 日志记录详细规范
