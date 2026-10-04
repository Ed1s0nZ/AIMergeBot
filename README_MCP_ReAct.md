> 旧版本参考：当前服务已采用 Eino 和 `/api/v1` 工作台。请以 [README](README.md) 为准；本文中的旧工具和流程不作为当前运行契约。

# AIMergeBot MCP & ReAct 功能说明

## 概述

AIMergeBot 现已支持 MCP（Model Context Protocol）和 ReAct（Reasoning and Acting）功能，提供更智能的代码安全审计能力。

## 新功能特性

### 1. MCP（Model Context Protocol）支持

MCP 允许大模型通过工具调用访问代码仓库，实现更深入的代码分析。

#### 可用工具

- **read_file**: 读取指定文件的完整内容
- **list_files**: 列出指定目录下的所有文件
- **search_code**: 在代码中搜索指定的文本或模式
- **git_diff**: 获取指定提交或分支的代码差异
- **git_log**: 获取Git提交历史

#### 工具调用示例

```json
{
  "tool_name": "read_file",
  "arguments": {
    "file_path": "src/main.go"
  }
}
```

### 2. ReAct（Reasoning and Acting）审计

ReAct 方法通过多轮推理和工具调用来进行代码安全分析：

1. **思考（Thought）**: 分析当前情况，确定下一步行动
2. **行动（Action）**: 调用相应的MCP工具获取信息
3. **观察（Observation）**: 获取工具执行结果
4. **重复**: 基于观察结果继续推理，直到得出最终结论

#### ReAct 推理流程

```
思考 → 工具调用 → 观察结果 → 思考 → ... → 最终答案
```

## 配置说明

### 配置文件更新

在 `config.yaml` 中添加以下配置：

```yaml
# MCP和ReAct配置
mcp:
  # 是否启用MCP功能
  enabled: true
  # 临时仓库存储路径（可选，默认使用系统临时目录）
  temp_repo_path: ""
  # 最大推理步骤数
  max_steps: 10
  # 是否启用详细日志
  verbose_logging: true

react:
  # 是否启用ReAct审计（替代传统AI分析）
  enabled: true
  # ReAct模型配置
  model: "gpt-4o-mini"
  # 温度参数（0.0-1.0）
  temperature: 0.1
  # 最大重试次数
  max_retries: 3
```

### 配置选项说明

- **mcp.enabled**: 是否启用MCP功能
- **mcp.max_steps**: ReAct推理的最大步骤数
- **react.enabled**: 是否启用ReAct审计（替代传统方法）
- **react.model**: 用于ReAct推理的模型
- **react.temperature**: 模型温度参数，控制输出的随机性

## 使用方法

### 1. 启用ReAct审计

设置 `config.yaml` 中的 `react.enabled: true` 和 `mcp.enabled: true`。

### 2. 查看ReAct审计详情

在Web界面中，每个MR卡片都会显示"🧠 ReAct详情"链接，点击可查看详细的推理过程。

### 3. API接口

#### 获取ReAct审计结果

```
GET /react_audit/{project_id}/{mr_id}
```

#### 获取MCP工具列表

```
GET /mcp_tools
```

## 工作流程

### 传统方法 vs ReAct方法

**传统方法**:
1. 获取MR diff
2. 直接发送给大模型分析
3. 返回安全问题列表

**ReAct方法**:
1. 获取MR diff
2. 克隆代码仓库到本地
3. 大模型进行多轮推理：
   - 分析代码变更
   - 调用MCP工具获取更多上下文
   - 深入分析潜在风险
4. 生成最终的安全评估报告

### 优势对比

| 特性 | 传统方法 | ReAct方法 |
|------|----------|-----------|
| 上下文理解 | 有限 | 深入 |
| 工具调用 | 无 | 支持多种工具 |
| 推理过程 | 单次 | 多轮迭代 |
| 可解释性 | 低 | 高 |
| 准确性 | 中等 | 更高 |

## 技术实现

### 核心组件

1. **MCPExecutor**: 执行MCP工具调用
2. **ReActAuditor**: 管理ReAct推理过程
3. **Storage**: 存储ReAct审计结果
4. **Web界面**: 展示推理过程和结果

### 数据库表结构

新增 `react_audit_results` 表：

```sql
CREATE TABLE react_audit_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER,
    mr_iid INTEGER,
    react_result_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, mr_iid)
);
```

## 故障排除

### 常见问题

1. **ReAct审计失败**
   - 检查Git仓库访问权限
   - 确认MCP工具可用性
   - 查看日志获取详细错误信息

2. **工具调用失败**
   - 确认系统已安装git、grep等工具
   - 检查文件路径权限
   - 验证仓库克隆URL

3. **性能问题**
   - 调整 `max_steps` 参数
   - 优化临时目录配置
   - 考虑使用更快的模型

### 日志查看

启用详细日志后，可以在控制台看到：
- ReAct推理步骤
- MCP工具调用
- 错误信息和回退机制

## 未来计划

1. **更多MCP工具**: 添加代码分析、依赖检查等工具
2. **自定义工具**: 支持用户自定义MCP工具
3. **性能优化**: 缓存机制、并行处理
4. **可视化增强**: 更丰富的推理过程展示

## 贡献

欢迎提交Issue和Pull Request来改进MCP和ReAct功能。 