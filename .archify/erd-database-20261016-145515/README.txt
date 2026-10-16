Multica 数据库 ER 图谱

打开 index.html：按 31 个业务域浏览 ER 图，搜索表名或字段名，查看完整字段字典。
135 张 public 表 / 1,441 个物理字段 / 109 条 FK 约束（111 个列映射）。
另列出 50 组有源码证据的应用关联。

来源：提交 12724238803781e4a0800e3f5c7fd3cdc5cf7d1e。
615 份迁移通过仓库迁移程序在一次性 PostgreSQL 17 容器重放。
未读取生产数据库，未复制业务数据；临时数据库已经停止和删除。
pg_bigm / pg_cron 的可选迁移按仓库条件执行或跳过。

schema.sql / schema.json 保存完整结构。
relationships.json 保存外键列映射和应用关系证据。
每张表在所属分图完整列出全部字段；必要的外键目标也完整展开。
PK / FK 来自真实约束。UK 可能是复合键的组成字段。
条件唯一索引保留在字段字典中；动态 JSONB 模型和全部多态引用不在清单内。

31 张 ER 图通过 Archify validate / deliver / check / browser-check。
使用适合完整 schema 的 standard 模式，自动路由提示保留在 validation.json。
未逐张进行人工视觉验收。
总览通过浏览器交互、宽度溢出和字段完整性检查。

源码链接面向原仓库位置；离线分享包保留来源路径和行号，不包含源码仓库。
每张分域 HTML 独立运行；总览依赖同目录分图。
各域 candidate.json 是已验证的图表规格。
