BEGIN;

DO $$
DECLARE
  ws uuid := 'f4f46622-8ad4-48b6-8a30-cc72ea2aadb3';
  actor uuid := '31cfe93e-caf0-49d0-82b4-3ec569c76f86';
  runtime uuid;
  plan uuid;
  agent_tf uuid; agent_fx uuid; agent_js uuid; agent_bd uuid; agent_cs uuid; agent_zg uuid;
  squad_tf uuid; squad_fx uuid; squad_js uuid; squad_bd uuid; squad_cs uuid; squad_zg uuid;
  project_tf uuid; project_fx uuid; project_js uuid; project_bd uuid; project_cs uuid; project_zg uuid;
  issue_parent uuid;
  next_number int;
BEGIN
  SELECT id INTO runtime
  FROM agent_runtime
  WHERE workspace_id = ws AND provider = 'codex'
  ORDER BY status = 'online' DESC, created_at DESC
  LIMIT 1;

  IF runtime IS NULL THEN
    SELECT id INTO runtime
    FROM agent_runtime
    WHERE workspace_id = ws
    ORDER BY status = 'online' DESC, created_at DESC
    LIMIT 1;
  END IF;

  IF runtime IS NULL THEN
    RAISE EXCEPTION 'No agent_runtime found for workspace %', ws;
  END IF;

  DELETE FROM issue
  WHERE workspace_id = ws AND metadata->>'import_batch' = 'malaysia_sample_v1';

  DELETE FROM project
  WHERE workspace_id = ws AND title LIKE '[MY Sample] %';

  DELETE FROM squad
  WHERE workspace_id = ws AND name LIKE '[MY Sample] %';

  DELETE FROM agent
  WHERE workspace_id = ws AND name LIKE '[MY Sample] % Manager Agent';

  DELETE FROM milestone
  WHERE workspace_id = ws AND title = '[MY Sample] 马来西亚 H1 经营计划';

  INSERT INTO milestone (workspace_id, title, description, start_date, end_date, status, position, created_by)
  VALUES (
    ws,
    '[MY Sample] 马来西亚 H1 经营计划',
    '从马来西亚业务半年经营仪表板导入的结构验证样本。KPI 作为结果反馈独立于项目完成进度。',
    '2026-06-01',
    '2026-11-30',
    'in_progress',
    0,
    actor
  )
  RETURNING id INTO plan;

  INSERT INTO agent (workspace_id, name, description, runtime_mode, runtime_config, runtime_id, visibility, owner_id, instructions, status)
  VALUES
    (ws, '[MY Sample] 投放 Manager Agent', '经营计划样本：投放小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理投放小队任务，关注渠道投放、ASO、SEM、信息流和复贷营销。', 'offline'),
    (ws, '[MY Sample] 风险 Manager Agent', '经营计划样本：风险小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理风险小队任务，关注征信、反欺诈、策略引擎和模型验收。', 'offline'),
    (ws, '[MY Sample] 技术 Manager Agent', '经营计划样本：技术小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理技术小队任务，关注核心系统、支付、数据管道、模型服务和API网关。', 'offline'),
    (ws, '[MY Sample] BD Manager Agent', '经营计划样本：BD小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理BD小队任务，关注API合作、员工贷、供应商与监管拜访。', 'offline'),
    (ws, '[MY Sample] 催收 Manager Agent', '经营计划样本：催收小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理催收小队任务，关注外包、AI机器人、自建坐席和回款率。', 'offline'),
    (ws, '[MY Sample] 综管 Manager Agent', '经营计划样本：综管小队 Manager Agent', 'local', '{}'::jsonb, runtime, 'workspace', actor, '管理综管小队任务，关注办公室、银行账户、税务社保、监管材料和年审。', 'offline')
  ;

  SELECT id INTO agent_tf FROM agent WHERE workspace_id = ws AND name = '[MY Sample] 投放 Manager Agent';
  SELECT id INTO agent_fx FROM agent WHERE workspace_id = ws AND name = '[MY Sample] 风险 Manager Agent';
  SELECT id INTO agent_js FROM agent WHERE workspace_id = ws AND name = '[MY Sample] 技术 Manager Agent';
  SELECT id INTO agent_bd FROM agent WHERE workspace_id = ws AND name = '[MY Sample] BD Manager Agent';
  SELECT id INTO agent_cs FROM agent WHERE workspace_id = ws AND name = '[MY Sample] 催收 Manager Agent';
  SELECT id INTO agent_zg FROM agent WHERE workspace_id = ws AND name = '[MY Sample] 综管 Manager Agent';

  INSERT INTO squad (workspace_id, name, description, leader_id, creator_id, instructions)
  VALUES
    (ws, '[MY Sample] 投放小队', '映射 HTML 角色：投放。', agent_tf, actor, '按经营计划推进投放相关项目和任务。'),
    (ws, '[MY Sample] 风险小队', '映射 HTML 角色：风险。', agent_fx, actor, '按经营计划推进风险相关项目和任务。'),
    (ws, '[MY Sample] 技术小队', '映射 HTML 角色：技术。', agent_js, actor, '按经营计划推进技术相关项目和任务。'),
    (ws, '[MY Sample] BD小队', '映射 HTML 角色：BD。', agent_bd, actor, '按经营计划推进BD相关项目和任务。'),
    (ws, '[MY Sample] 催收小队', '映射 HTML 角色：催收。', agent_cs, actor, '按经营计划推进催收相关项目和任务。'),
    (ws, '[MY Sample] 综管小队', '映射 HTML 角色：综管。', agent_zg, actor, '按经营计划推进综管相关项目和任务。');

  SELECT id INTO squad_tf FROM squad WHERE workspace_id = ws AND name = '[MY Sample] 投放小队';
  SELECT id INTO squad_fx FROM squad WHERE workspace_id = ws AND name = '[MY Sample] 风险小队';
  SELECT id INTO squad_js FROM squad WHERE workspace_id = ws AND name = '[MY Sample] 技术小队';
  SELECT id INTO squad_bd FROM squad WHERE workspace_id = ws AND name = '[MY Sample] BD小队';
  SELECT id INTO squad_cs FROM squad WHERE workspace_id = ws AND name = '[MY Sample] 催收小队';
  SELECT id INTO squad_zg FROM squad WHERE workspace_id = ws AND name = '[MY Sample] 综管小队';

  INSERT INTO squad_member (squad_id, member_type, member_id, role)
  VALUES
    (squad_tf, 'agent', agent_tf, 'manager'),
    (squad_fx, 'agent', agent_fx, 'manager'),
    (squad_js, 'agent', agent_js, 'manager'),
    (squad_bd, 'agent', agent_bd, 'manager'),
    (squad_cs, 'agent', agent_cs, 'manager'),
    (squad_zg, 'agent', agent_zg, 'manager');

  INSERT INTO project (workspace_id, title, description, icon, status, priority, lead_type, lead_id, milestone_id)
  VALUES
    (ws, '[MY Sample] 投放增长计划', '来自 HTML 投放角色任务：应用上架、ASO、流量方签约、SEM、信息流与复贷营销。', '📣', 'in_progress', 'high', 'agent', agent_tf, plan),
    (ws, '[MY Sample] 风险策略与模型计划', '来自 HTML 风险角色任务：征信、基础风控、反欺诈、数据集市、老客策略与模型验收。', '🛡️', 'in_progress', 'high', 'agent', agent_fx, plan),
    (ws, '[MY Sample] 技术平台与数据计划', '来自 HTML 技术角色任务：核心系统、支付网关、数据管道、策略引擎、数据集市和模型 serving。', '⚙️', 'in_progress', 'high', 'agent', agent_js, plan),
    (ws, '[MY Sample] BD合作拓展计划', '来自 HTML BD角色任务：API合作方、MOU、员工贷、Vendor协议、监管拜访。', '🤝', 'planned', 'medium', 'agent', agent_bd, plan),
    (ws, '[MY Sample] 催收运营计划', '来自 HTML 催收角色任务：外包机构、AI催收机器人、M0/M1/M2/M3+流程和回款率。', '☎️', 'planned', 'medium', 'agent', agent_cs, plan),
    (ws, '[MY Sample] 综管支撑计划', '来自 HTML 综管角色任务：办公室、对公账户、税务社保、监管报备、年审和审计资料。', '🏢', 'planned', 'medium', 'agent', agent_zg, plan);

  SELECT id INTO project_tf FROM project WHERE workspace_id = ws AND title = '[MY Sample] 投放增长计划';
  SELECT id INTO project_fx FROM project WHERE workspace_id = ws AND title = '[MY Sample] 风险策略与模型计划';
  SELECT id INTO project_js FROM project WHERE workspace_id = ws AND title = '[MY Sample] 技术平台与数据计划';
  SELECT id INTO project_bd FROM project WHERE workspace_id = ws AND title = '[MY Sample] BD合作拓展计划';
  SELECT id INTO project_cs FROM project WHERE workspace_id = ws AND title = '[MY Sample] 催收运营计划';
  SELECT id INTO project_zg FROM project WHERE workspace_id = ws AND title = '[MY Sample] 综管支撑计划';

  SELECT COALESCE(MAX(number), 0) + 1 INTO next_number FROM issue WHERE workspace_id = ws;

  INSERT INTO issue (workspace_id, title, description, status, priority, assignee_type, assignee_id, creator_type, creator_id, parent_issue_id, position, start_date, due_date, number, project_id, metadata)
  VALUES (ws, '应用商店上线 + ASO报告', '关键交付：应用商店上线截图 + 基础ASO报告。', 'in_progress', 'high', 'squad', squad_tf, 'member', actor, NULL, 10, '2026-07-01', '2026-07-31', next_number, project_tf, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"投放","month":2,"key_delivery":true,"original_progress":0}'::jsonb)
  RETURNING id INTO issue_parent;
  next_number := next_number + 1;
  INSERT INTO issue (workspace_id, title, description, status, priority, assignee_type, assignee_id, creator_type, creator_id, parent_issue_id, position, start_date, due_date, number, project_id, metadata)
  VALUES
    (ws, '商店物料本地化', '标题、描述、关键词马来语/英语本地化。', 'todo', 'medium', 'squad', squad_tf, 'member', actor, issue_parent, 11, '2026-06-01', '2026-06-30', next_number, project_tf, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"投放","month":1,"parent":"应用商店上线 + ASO报告"}'::jsonb),
    (ws, '上线截图与ASO报告归档', '输出上线截图和基础ASO报告。', 'todo', 'medium', 'squad', squad_tf, 'member', actor, issue_parent, 12, '2026-07-01', '2026-07-31', next_number + 1, project_tf, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"投放","month":2,"parent":"应用商店上线 + ASO报告"}'::jsonb);
  next_number := next_number + 2;

  INSERT INTO issue (workspace_id, title, description, status, priority, assignee_type, assignee_id, creator_type, creator_id, position, start_date, due_date, number, project_id, metadata)
  VALUES
    (ws, '反欺诈规则集部署（≥5条）', '规则集上线；确立通过率初始基准值（约30%）。', 'todo', 'high', 'squad', squad_fx, 'member', actor, 20, '2026-08-01', '2026-08-31', next_number, project_fx, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"风险","month":3,"key_delivery":true,"kpi_hint":"通过率初始基准值约30%"}'::jsonb),
    (ws, '风控策略引擎交付', '支持规则热部署，风险同事可自行配置规则。', 'todo', 'high', 'squad', squad_js, 'member', actor, 30, '2026-08-01', '2026-08-31', next_number + 1, project_js, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"技术","month":3,"key_delivery":true}'::jsonb),
    (ws, '签署1份MOU', 'MOU明确技术对接意向与排他性讨论。', 'todo', 'medium', 'squad', squad_bd, 'member', actor, 40, '2026-07-01', '2026-07-31', next_number + 2, project_bd, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"BD","month":2,"key_delivery":true}'::jsonb),
    (ws, 'AI催收机器人配置', '印尼系统接入 + 马来语话术测试完成。', 'todo', 'medium', 'squad', squad_cs, 'member', actor, 50, '2026-07-01', '2026-07-31', next_number + 3, project_cs, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"催收","month":2,"key_delivery":true}'::jsonb),
    (ws, '监管报备材料递交', '公司执照、董事信息、反洗钱政策的公证与递交。', 'todo', 'medium', 'squad', squad_zg, 'member', actor, 60, '2026-09-01', '2026-09-30', next_number + 4, project_zg, '{"import_batch":"malaysia_sample_v1","source":"malaysia_html","role":"综管","month":4,"key_delivery":true}'::jsonb);

  RAISE NOTICE 'Imported Malaysia sample plan %, runtime %', plan, runtime;
END $$;

COMMIT;
