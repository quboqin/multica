import json,re,pathlib,html,collections
root=pathlib.Path.cwd();out=root/'.archify/erd-database-20261016-145515';schema=json.loads((out/'schema.json').read_text());tables={t['name']:t for t in schema['tables']}
models=(root/'server/pkg/db/generated/models.go').read_text();modelmap={}
for m in re.finditer(r'^type (\w+) struct \{(.*?)^\}',models,re.M|re.S):
 modelmap[m[1].lower()]={'path':'server/pkg/db/generated/models.go','line':models[:m.start()].count('\n')+1,'end_line':models[:m.end()].count('\n')+1}
create={};migrationrefs=collections.defaultdict(list)
for p in sorted((root/'server/migrations').glob('*.up.sql')):
 text=p.read_text()
 for m in re.finditer(r'CREATE TABLE\s+(?:IF NOT EXISTS\s+)?"?(\w+)"?\s*\(',text,re.I):
  end=text.find(';',m.start());create[m[1]]={'path':str(p.relative_to(root)),'line':text[:m.start()].count('\n')+1,'end_line':text[:end].count('\n')+1}
 for t in tables:
  if re.search(r'\b'+re.escape(t)+r'\b',text):migrationrefs[t].append(str(p.relative_to(root)))
# Read explicit qualified equality joins only; ambiguous aliases and non-id targets are excluded.
logical={}
for p in sorted((root/'server/pkg/db/queries').glob('*.sql')):
 text=p.read_text()
 starts=list(re.finditer(r'^-- name:.*$',text,re.M))
 for j,start in enumerate(starts):
  end=starts[j+1].start() if j+1<len(starts) else len(text);block=text[start.start():end]
  aliases=collections.defaultdict(set)
  for m in re.finditer(r'\b(?:FROM|JOIN)\s+"?(\w+)"?(?:\s+(?:AS\s+)?(\w+))?',block,re.I):
   table,alias=m.groups()
   if table not in tables:continue
   aliases[table].add(table)
   if alias and alias.upper() not in {'WHERE','ON','JOIN','LEFT','RIGHT','INNER','ORDER','GROUP','LIMIT','UNION','SET','RETURNING','CROSS','FULL'}:aliases[alias].add(table)
  for m in re.finditer(r'(\w+)\.(\w+)\s*=\s*(\w+)\.(\w+)',block):
   aa,ac,ba,bc=m.groups()
   if len(aliases[aa])!=1 or len(aliases[ba])!=1:continue
   at=next(iter(aliases[aa]));bt=next(iter(aliases[ba]))
   if at==bt:continue
   for src,col,target,targetcol in [(at,ac,bt,bc),(bt,bc,at,ac)]:
    if targetcol!='id' or col=='id' or col not in {c['name'] for c in tables[src]['columns']}:continue
    if 'id' not in {c['name'] for c in tables[target]['columns']}:continue
    line=text[:start.start()+m.start()].count('\n')+1
    logical.setdefault((src,col,target,'id'),{'path':str(p.relative_to(root)),'line':line,'end_line':line,'expression':m[0]})
physical=[]
for t in tables.values():
 for c in t['constraints']:
  if c['kind']=='f':
   m=re.search(r'FOREIGN KEY \((.*?)\) REFERENCES (?:public\.)?"?(\w+)"?\((.*?)\)',c['definition'])
   assert m,c
   srcs=[x.strip(' "') for x in m[1].split(',')];tgts=[x.strip(' "') for x in m[3].split(',')]
   for a,b in zip(srcs,tgts):physical.append((t['name'],a,m[2],b))
physicalset=set(physical);logical={k:v for k,v in logical.items() if k not in physicalset}
# Additional relations verified at the actual write / delete sites.
for a,b,c,path,lo,hi in [
 ('plugin_package_version','package_id','plugin_package','server/internal/service/plugin_package.go',133,150),
 ('plugin_package_file','version_id','plugin_package_version','server/internal/service/plugin_package.go',157,167),
 ('plugin_installation','package_version_id','plugin_package_version','server/pkg/db/queries/plugin.sql',66,72),
]: logical[(a,b,c,'id')]=dict(path=path,line=lo,end_line=hi,expression='verified parent identifier')
conditions={('record_link','to_id','record','id'):"to_type=record",('record_link','to_id','issue','id'):"to_type=issue",('issue','assignee_id','agent','id'):"assignee_type=agent",('issue','assignee_id','squad','id'):"assignee_type=squad",('squad_member','member_id','agent','id'):"member_type=agent",('issue_wakeup','filter_actor_id','agent','id'):"filter_actor_type=agent"}
for k,condition in conditions.items():
 if k in logical:logical[k]['condition']=condition
groups=[
 ('accounts','账号与凭据','user personal_access_token verification_code'),
 ('workspaces','工作区与成员','workspace member workspace_invitation workspace_share_link seat_capacity_outbox'),
 ('projects','项目','project project_resource'),
 ('issues','任务与标签','issue issue_label issue_to_label issue_dependency'),
 ('issue-config','任务配置与视图','issue_status issue_property quick_action issue_view issue_view_preference'),
 ('comments','评论与订阅','comment comment_reaction issue_reaction issue_subscriber'),
 ('documents','文档与权限','document_access document_collaborator document_version document_publication comment_agent_grant'),
 ('collections','Collections','collection collection_field record record_link collection_access collection_collaborator'),
 ('agents','智能体配置','agent agent_invocation_target agent_builder_draft agent_to_label agent_mcp_server workspace_mcp_server'),
 ('skills','技能与团队','skill skill_file agent_skill skill_to_label squad squad_member'),
 ('runtime','运行时与守护进程','agent_runtime runtime_profile daemon_connection daemon_token'),
 ('runs','运行队列与消息','agent_task_queue task_message task_token'),
 ('supplements','运行补充与来源','task_supplement task_supplement_capability issue_source_context issue_source_context_object_intent'),
 ('wakeup','任务唤醒与子事件','issue_wakeup issue_wakeup_receipt issue_child_event'),
 ('usage','运行用量统计','task_usage task_usage_hourly task_usage_hourly_dirty task_usage_hourly_rollup_state'),
 ('chat','对话','chat_session chat_message chat_pinned_agent chat_draft_restore'),
 ('autopilot','自动化规则与运行','autopilot autopilot_trigger autopilot_run autopilot_rule_version'),
 ('autopilot-access','自动化权限与配额','autopilot_collaborator autopilot_subscriber autopilot_quota_period autopilot_quota_reservation'),
 ('plugin-packages','插件包与安装','plugin_package plugin_package_version plugin_package_file plugin_installation'),
 ('plugin-runtime','插件执行与存储','plugin_invocation plugin_hook_schedule plugin_storage plugin_secret'),
 ('channels','渠道接入与会话','channel_installation channel_user_binding channel_binding_token channel_chat_session_binding channel_chat_context_generation'),
 ('channel-delivery','渠道消息交付','channel_reply_delivery channel_task_delivery channel_outbound_message channel_outbound_card_message channel_media_pending_object'),
 ('channel-audit','渠道审计与钉钉','channel_inbound_audit channel_inbound_message_dedup dingtalk_bot_identity dingtalk_group_presence dingtalk_group_route'),
 ('lark','Lark 专用接入','lark_installation lark_user_binding lark_binding_token lark_chat_session_binding'),
 ('lark-messages','Lark 消息与审计','lark_inbound_audit lark_inbound_message_dedup lark_outbound_card_message'),
 ('github','GitHub 接入与 PR','github_installation github_pending_installation github_pull_request issue_pull_request issue_pull_request_exclusion issue_pr_automation'),
 ('github-checks','GitHub 检查与 Webhook','github_pull_request_check_suite github_pull_request_check_run github_pending_check_suite webhook_delivery'),
 ('vcs','通用 VCS','vcs_connection vcs_pull_request vcs_commit_status issue_vcs_pull_request'),
 ('personal','个人状态与通知','inbox_item pinned_item notification_preference feedback client_usage_daily contact_sales_inquiry'),
 ('operations','运维与索引','activity_log maintenance_job sys_cron_executions search_index_change search_index_prune_mark schema_migrations instance_telemetry_state'),
 ('assets','附件与外部账号','attachment user_composio_connection')
]
flat=[t for _,_,ts in groups for t in ts.split()];assert len(flat)==len(set(flat));assert set(flat)==set(tables),(set(tables)-set(flat),set(flat)-set(tables))
def keys(t,col):
 out=[]
 for ix in t['indexes']:
  if col in (ix['columns'] or []):
   if ix['primary']:out.append('pk')
   elif ix['unique'] and not ix['predicate'] and ix['valid']:out.append('uk')
 if any(a==t['name'] and b==col for a,b,c,d in physical):out.append('fk')
 return list(dict.fromkeys(out))
def entity(name,row,col,tag):
 t=tables[name];attrs=[]
 for c in t['columns']:
  typ=c['type']
  # Preserve SQL types; append Chinese source comments where present.
  if c['comment'] and re.search('[\u4e00-\u9fff]',c['comment']):typ+='｜'+c['comment']
  a={'name':c['name'],'type':typ};k=keys(t,c['name'])
  if k:a['key']=k[0] if len(k)==1 else k
  fk=next((x for x in physical if x[0]==name and x[1]==c['name']),None)
  if fk:a['references']=fk[2]+'.'+fk[3]
  attrs.append(a)
 sources=[]
 if name in create:sources.append(create[name])
 model=modelmap.get(name.replace('_','').lower())
 if model:sources.append(model)
 if name=='schema_migrations':
  path=root/'server/cmd/migrate/main.go';txt=path.read_text();m=re.search(r'CREATE TABLE IF NOT EXISTS',txt);line=txt[:m.start()].count('\n')+1;sources=[dict(path=str(path.relative_to(root)),line=line,end_line=line+4)]
 assert sources,name
 width=max(300,max(54+len(a['name'])*6.6+len(a['type'])*6.3+20 for a in attrs))
 return dict(id=name,label=name,tag=tag,row=row,col=col,width=round(width),attributes=attrs,sources=sources)
def candidate(slug,title,names):
 # Balanced blocks keep every table in one domain rectangular; an odd-sized group uses one row.
 count=len(names);cols=count if count%2 else min(3,count//2)
 if count==6:cols=3
 if count==4:cols=2
 if count==7:cols=7
 ents=[entity(n,i//cols,i%cols,title) for i,n in enumerate(names)]
 rel=[]
 for a in names:
  for fk in tables[a]['constraints']:
   if fk['kind']!='f':continue
   match=re.search(r'FOREIGN KEY \((.*?)\) REFERENCES (?:public\.)?"?(\w+)"?\((.*?)\)',fk['definition']);c=match[2]
   if c not in names or a==c:continue
   columns=fk['columns'];one=any(ix['unique'] and not ix['predicate'] and set(ix['columns'] or [])==set(columns) for ix in tables[a]['indexes'])
   nullable=any(x['nullable'] for x in tables[a]['columns'] if x['name'] in columns)
   rel.append(dict(id=fk['name'],**{'from':a,'to':c},label=', '.join(columns),fromCardinality='one' if one else 'many',toCardinality='one',fromOptional=True,toOptional=nullable))
 # Explicit query equality proves association, not a foreign-key constraint.
 for (a,b,c,d),src in logical.items():
  if a in names and c in names and a!=c and not any(x['from']==a and x['to']==c and x['label']==b for x in rel):
   one=any(ix['unique'] and not ix['predicate'] and ix['columns']==[b] for ix in tables[a]['indexes'])
   nullable=next(x for x in tables[a]['columns'] if x['name']==b)['nullable']
   rel.append(dict(id=f'logical_{a}_{b}_{c}',**{'from':a,'to':c},label=b+(' ['+src['condition']+']' if 'condition' in src else '')+' · 应用关联',fromCardinality='one' if one else 'many',toCardinality='one',fromOptional=True,toOptional=nullable,identifying=False))
   e=next(x for x in ents if x['id']==a)
   if len(e['sources'])<3:e['sources'].append({k:v for k,v in src.items() if k in {'path','line','end_line'}})
 cards=[dict(dot='cyan',title='范围与来源',items=[f'本分图完整展示 {count} 张表、{sum(len(tables[n]["columns"]) for n in names)} 个物理字段。来自当前提交的 615 份迁移在隔离 PostgreSQL 17 中重放后的 schema。','跨域外键保留在字段 references 中；其他业务域见总览。字典另含 NULL、默认值、注释、索引及约束定义。']),dict(dot='amber',title='如何阅读键与连线',items=['PK 为真实主键；FK 为实际数据库外键；UK 为无条件唯一键或复合唯一键的组成字段，不表示复合键的每个字段单独唯一。','实线表示实际 FK；虚线仅表示 SQL 中已核实的应用关联，数据库不保证引用完整性。关系端点表示业务上限；逻辑引用仍可能出现悬空值。','部分唯一索引不标为无条件 UK，其完整谓词保留在数据字典中。自引用保留在字段信息中，不绘制回环。'])]
 foreign=[f'{a}.{b} → {c}.{d}' for a,b,c,d in physical if a in names and c not in names]
 if foreign:cards.append(dict(dot='violet',title='跨域数据库外键',items=foreign))
 composite=[ix['definition'] for n in names for ix in tables[n]['indexes'] if ix['unique'] and (len(ix['columns'] or [])>1 or ix['predicate'])]
 if composite:cards.append(dict(dot='emerald',title='复合键与条件唯一性',items=composite))
 return dict(schema_version=1,diagram_type='erd',meta=dict(title='Multica 数据库 · '+title,locale='zh-CN',quality_profile='standard',output=str((out/slug/(slug+'.html')).relative_to(root)),repository=dict(url='git@github.com:quboqin/multica.git',revision=schema['revision'],link_mode='local-only')),layout=dict(mode='grid',gapX=95,gapY=55),entities=ents,relationships=rel,cards=cards)
for slug,title,ts in groups:
 p=out/slug;p.mkdir(exist_ok=True)
 (p/'candidate.json').write_text(json.dumps(candidate(slug,title,ts.split()),ensure_ascii=False,indent=2)+'\n')
(out/'relationships.json').write_text(json.dumps({'physical':[dict(source=a,column=b,target=c,target_column=d) for a,b,c,d in physical],'logical':[dict(source=a,column=b,target=c,target_column=d,evidence=s) for (a,b,c,d),s in logical.items()]},ensure_ascii=False,indent=2)+'\n')
(out/'atlas-manifest.json').write_text(json.dumps({'groups':[dict(slug=s,title=t,tables=n.split()) for s,t,n in groups],'tables':len(tables),'columns':sum(len(t['columns']) for t in tables.values()),'physical_foreign_keys':sum(sum(c['kind']=='f' for c in t['constraints']) for t in tables.values()),'explicit_query_associations':len(logical),'revision':schema['revision']},ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'domains':len(groups),'physical':len(physical),'query_associations':len(logical),'missing_models':[n for n in tables if n.replace('_','').lower() not in modelmap]}))
