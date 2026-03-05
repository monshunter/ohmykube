# API 定义规范

## 1 目录结构

```
docs/apis/
├── README.md                    # 本文件，规范说明
├── INDEX.md                     # 文档索引
├── ${service}-openapi.json      # OpenAPI 规格
├── ${subject}.schema.json       # JSON Schema
└── ${service}-api.json          # 自定义 API 定义
```

## 2 文件格式

所有 API 定义文件使用 JSON 格式，必须包含版本信息：

```json
{
  "version": "1.0.0",
  "info": {
    "title": "API 名称",
    "description": "API 描述"
  }
}
```

## 3 命名规范

| 类型 | 命名模式 | 示例 |
|------|----------|------|
| OpenAPI 规格 | `${service}-openapi.json` | `generator-openapi.json` |
| JSON Schema | `${subject}.schema.json` | `config.schema.json` |
| 接口定义 | `${service}-api.json` | `export-api.json` |

## 4 版本管理

使用语义化版本号（SemVer）：`MAJOR.MINOR.PATCH`

| 变更类型 | 版本号变化 | 示例 |
|----------|------------|------|
| 重大变更（不兼容） | MAJOR +1 | 1.0.0 → 2.0.0 |
| 新增功能（向后兼容） | MINOR +1 | 1.0.0 → 1.1.0 |
| Bug 修复 | PATCH +1 | 1.0.0 → 1.0.1 |

## 5 文档模板

### 5.1 OpenAPI 规格模板

```json
{
  "openapi": "3.0.0",
  "info": {
    "title": "Service Name API",
    "version": "1.0.0",
    "description": "API 描述"
  },
  "paths": {
    "/endpoint": {
      "get": {
        "summary": "端点描述",
        "responses": {
          "200": {
            "description": "成功响应"
          }
        }
      }
    }
  }
}
```

### 5.2 JSON Schema 模板

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "version": "1.0.0",
  "title": "Schema 名称",
  "description": "Schema 描述",
  "type": "object",
  "properties": {
    "field": {
      "type": "string",
      "description": "字段描述"
    }
  },
  "required": ["field"]
}
```

## 6 检查清单

完成 API 定义前，确认以下事项：

- [ ] JSON 格式有效（可通过 JSON 验证工具检查）
- [ ] 包含版本信息
- [ ] 文件命名符合规范
- [ ] 已更新 `INDEX.md` 索引
