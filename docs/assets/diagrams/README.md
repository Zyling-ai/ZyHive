# 图表维护

本目录的 16 组架构图由 [`scripts/generate_docs_diagrams.py`](../../../scripts/generate_docs_diagrams.py) 统一生成。每组包含：

- `.svg`：文档直接引用的无脚本、可访问静态图，包含 `<title>` 与 `<desc>`；
- `.mmd`：同一语义结构的 Mermaid 源，便于审阅流程和二次编辑。

修改图中节点或说明时，编辑生成脚本中的 `DIAGRAMS`，然后运行：

```bash
make docs-diagrams
make docs
```

不要直接手改生成的 SVG。图表只表达关键关系，不替代正文中的一致性语义、安全边界和失败处理说明。
