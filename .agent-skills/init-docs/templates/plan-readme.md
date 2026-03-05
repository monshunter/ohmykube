# 计划文档规范

## 1 目录结构

```
docs/plan/
├── README.md                              # 本文件，规范说明
├── INDEX.md                               # 文档索引
└── ${subject}/                            # 计划目录
    ├── context.yaml                       # 上下文声明（必填，/implement 入口）
    ├── implementation.md                  # 实施计划
    ├── implementation-checklist.md        # 实施 Checklist
    ├── ${type}-plan.md                    # 其他计划（如测试计划）
    └── ${type}-plan-checklist.md          # 对应 Checklist
```

**示例**：
```
docs/plan/
├── feature-auth/
│   ├── context.yaml
│   ├── implementation.md
│   ├── implementation-checklist.md
│   ├── unit-test-plan.md
│   └── unit-test-plan-checklist.md
└── ...
```

## 2 核心规则

1. **成对创建**：每个计划文档必须有对应的 Checklist
2. **版本一致**：计划和 Checklist 版本号必须一致
3. **唯一真理**：Checklist 是任务完成状态的唯一真理来源
4. **原子更新**：修改计划内容时，必须同步更新 Checklist
5. **并行优先**：默认使用 `parallel` 执行模式
6. **串行可选**：当计划不适合并行时，可使用 `sequential` 执行模式
7. **上下文声明**：每个计划目录必须包含 `context.yaml`（`/implement` 的上下文入口）

## 3 文档元信息

所有计划文档必须在头部包含元信息：

```markdown
> **版本**: 1.0
> **状态**: active
> **更新日期**: YYYY-MM-DD
> **执行模式**: parallel
```

**状态值说明**：

| 状态 | 含义 |
|------|------|
| `draft` | 草稿，尚未正式生效 |
| `active` | 生效中，正在执行 |
| `completed` | 已完成 |
| `superseded` | 已被取代，需注明新文档路径 |
| `deprecated` | 已废弃，不再适用 |

**执行模式说明**：

| 值 | 含义 | 适用场景 |
|---|---|---|
| `parallel` | 使用 Wave 分组，同 Wave 内 Phase 可并行 | 默认方案 |
| `sequential` | Phase 按编号顺序执行 | 强顺序依赖/小规模变更/并行收益低 |

> 新建计划必须显式写 `执行模式`（`parallel` 或 `sequential`），推荐优先 `parallel`。

## 4 命名规范

| 类型 | 计划文档 | Checklist |
|------|----------|-----------|
| 实施计划 | `implementation.md` | `implementation-checklist.md` |
| 测试计划 | `unit-test-plan.md` / `e2e-test-plan.md` | `unit-test-plan-checklist.md` / `e2e-test-plan-checklist.md` |
| 其他计划 | `${type}-plan.md` | `${type}-plan-checklist.md` |

## 5 文档模板（默认并行）

### 5.1 并行计划文档模板

```markdown
# 计划名称

> **版本**: 1.0
> **状态**: active
> **更新日期**: YYYY-MM-DD
> **执行模式**: parallel

**关联 Checklist**: [checklist](./implementation-checklist.md)

## 1 目标

描述本计划要达成的目标。

## 2 背景

说明为什么需要这个计划。

## 3 并行执行 DAG

| Phase | 描述 | Agent 角色 | 依赖 | 文件范围 | 预估项数 |
|-------|------|-----------|------|----------|----------|
| W0.Design | 接口契约确认 | lead | — | docs/ | 3 |
| W1.Auth | 认证基础设施 | apiserver-auth | W0 | internal/apiserver/auth/ | 6 |
| W1.Frontend | 前端登录页 | frontend | W0 | ui/src/ | 5 |
| W1.Config | 配置与 RBAC | config | W0 | config/ | 4 |
| W2.Service | Service 层集成 | apiserver-service | W1.Auth | internal/apiserver/grpc/, internal/apiserver/http/ | 5 |
| W2.Test | 场景测试 | test | W1.Auth, W1.Frontend | test/scenarios/ | 6 |
| W3.Verify | 端到端验收 | lead | W2.* | — | 3 |

## 4 实施步骤

### W0.Design: 接口契约确认
<!-- agent: lead -->
<!-- files: docs/ -->
<!-- depends-on: — -->
<!-- est-items: 3 -->

#### W0.Design.1 任务描述

具体步骤描述。

### W1.Auth: 认证基础设施
<!-- agent: apiserver-auth -->
<!-- files: internal/apiserver/auth/** -->
<!-- depends-on: W0 -->
<!-- est-items: 6 -->

#### W1.Auth.1 任务描述

具体步骤描述。

## 5 风险与应对

| 风险 | 应对措施 |
|------|----------|
| 风险 1 | 措施 1 |

## 6 关联文档

- [相关设计](../spec/xxx.md)
```

### 5.2 并行 Checklist 模板

```markdown
# 计划名称 Checklist

> **版本**: 1.0
> **状态**: active
> **更新日期**: YYYY-MM-DD

**关联计划**: [计划文档](./implementation.md)

## W0.Design: 接口契约确认 [lead]

- [ ] W0.Design.1 冻结接口契约与错误语义（文档）
- [ ] W0.Design.2 产出角色拆分与依赖图
- [ ] W0.Design.3 冻结验收标准与回归范围

## W1.Auth: 认证基础设施 [apiserver-auth]

- [ ] W1.Auth.1 HTTP 客户端工厂
- [ ] W1.Auth.2 OIDC Discovery Resolver
- [ ] W1.Auth.3 OIDC JWKS Resolver

## W1.Frontend: 前端登录页 [frontend]（与 W1.Auth 并行）

- [ ] W1.Frontend.1 Auth Context
- [ ] W1.Frontend.2 登录页组件

## W1.Config: 配置与 RBAC [config]（与 W1.Auth 并行）

- [ ] W1.Config.1 配置校验
- [ ] W1.Config.2 RBAC 更新

## W2.Service: Service 层集成 [apiserver-service]

- [ ] W2.Service.1 gRPC 集成
- [ ] W2.Service.2 HTTP 集成

## W2.Test: 场景测试 [test]（与 W2.Service 并行）

- [ ] W2.Test.1 测试场景编写
- [ ] W2.Test.2 测试运行与验证

## W3.Verify: 端到端验收 [lead]

- [ ] W3.Verify.1 全量测试运行
- [ ] W3.Verify.2 结果合成与文档更新
```

**格式约定**：

- Section 标题格式：`## {PhaseID}: 描述 [{角色ID}]`
- 同 Wave 的非首个 Phase 追加 `（与 W1.XXX 并行）` 标注
- 任务 ID 格式：`{PhaseID}.{序号}`

### 5.3 测试 Checklist 模板（含 phase-mapping）

当测试 checklist 通过 `testChecklist` 字段关联到实施 target 时，每个 section 标题后添加 `<!-- phase-mapping: -->` 注释，声明该 section 对应实施 checklist 的哪个 phase：

**串行模式**（实施 checklist 按 `## N` 分组）：

```markdown
## 1 登录页测试
<!-- phase-mapping: 2 -->

- [ ] 1.1 渲染模式切换测试
- [ ] 1.2 本地登录表单校验

## 2 注册页面测试
<!-- phase-mapping: 3 -->

- [ ] 2.1 Token 解析测试
- [ ] 2.2 密码校验测试
```

**并行模式**（实施 checklist 按 `## W{n}.{Phase}` 分组）：

```markdown
## 1 密码模块测试
<!-- phase-mapping: W1.Auth -->

- [ ] 1.1 Argon2id 哈希测试
- [ ] 1.2 密码验证测试

## 2 配置校验测试
<!-- phase-mapping: W1.Config -->

- [ ] 2.1 必填字段校验
```

**规则**：

- `<!-- phase-mapping: {impl-phase-id} -->` 紧跟 section heading 后
- 一个 test section 映射到一个 impl phase
- 多个 test section 可映射到同一个 impl phase
- `/tdd --test-checklist` 在每个 impl phase 完成后自动执行映射的 test section

### 5.4 串行模式适用条件

当满足以下任一条件时，可选择 `sequential`：

- 任务具有强顺序依赖，难以拆分为可并行 Wave
- 变更规模较小，拆分并行的协调成本高于收益
- 文件集中在单角色域，且不存在并行交付价值

使用 `sequential` 时，仍需保持本规范的命名、元信息和 Checklist 关联要求。

## 6 上下文声明（context.yaml）

每个计划目录必须包含 `context.yaml`，声明该计划的可执行目标与所需上下文文件。`/implement` 仅解析 `context.yaml`，不从 plan/spec 文档头部提取关联链接。

### 6.1 最小模板

```yaml
apiVersion: ferry.agent.context/v1alpha1
kind: PlanContext
metadata:
  name: ${subject}
spec:
  defaultTarget: backend
  targets:
    backend:
      plan: ./implementation.md
      checklist: ./implementation-checklist.md
      spec: ../../spec/${subject}-design.md
```

含测试计划的模板：

```yaml
apiVersion: ferry.agent.context/v1alpha1
kind: PlanContext
metadata:
  name: ${subject}
spec:
  defaultTarget: backend
  targets:
    backend:
      plan: ./implementation.md
      checklist: ./implementation-checklist.md
      spec: ../../spec/${subject}-design.md
      testPlan: ./unit-test-plan.md
      testChecklist: ./unit-test-plan-checklist.md
```

### 6.2 字段约束

| 字段 | 必填 | 说明 |
|------|------|------|
| `apiVersion` | 是 | 固定 `ferry.agent.context/v1alpha1` |
| `kind` | 是 | 固定 `PlanContext` |
| `metadata.name` | 是 | 计划标识，与目录名一致 |
| `spec.defaultTarget` | 是 | 默认执行目标 |
| `spec.targets` | 是 | 目标集合 |
| `spec.targets.<target>.plan` | 是 | 目标主计划文档（相对路径） |
| `spec.targets.<target>.checklist` | 是 | 目标执行 checklist（相对路径） |
| `spec.targets.<target>.spec` | 否 | 关联设计文档（相对路径） |
| `spec.targets.<target>.testPlan` | 否 | 目标关联测试计划文档（相对路径） |
| `spec.targets.<target>.testChecklist` | 否 | 目标关联测试 checklist（相对路径） |
| `spec.targets.<target>.references` | 否 | 其他只读引用文件路径列表（字符串数组） |

### 6.3 路径安全

所有路径归一化后必须位于 `docs/` 目录内，否则 `/implement` 校验将阻断。

## 7 检查清单

完成计划文档前，确认以下事项：

- [ ] 创建计划目录 `docs/plan/${subject}/`
- [ ] 创建 `context.yaml` 上下文声明
- [ ] 计划与 Checklist 成对创建
- [ ] 两个文档版本号一致
- [ ] 元信息包含 `执行模式` 字段（`parallel` 或 `sequential`）
- [ ] 若 `执行模式: parallel`，包含「并行执行 DAG」摘要表
- [ ] 若 `执行模式: parallel`，Phase 标题采用 `W{wave}.{Domain}` 格式
- [ ] 若 `执行模式: parallel`，每个 Phase 标题后包含机器可读 HTML 注释
- [ ] 若 `执行模式: parallel`，Checklist 任务 ID 使用 `{PhaseID}.{序号}`
- [ ] 若 `执行模式: sequential`，实施步骤使用顺序 Phase 编号（如 `3.1 Phase 1`）
- [ ] 已更新 `INDEX.md` 索引
- [ ] 计划文档关联了 Checklist
- [ ] Checklist 关联了计划文档
