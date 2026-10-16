import json,pathlib,copy,re,collections,math
root=pathlib.Path.cwd();out=root/'.archify/erd-database-20261016-145515'
manifest=json.loads((out/'atlas-manifest.json').read_text());schema=json.loads((out/'schema.json').read_text());tables={x['name']:x for x in schema['tables']};results=json.loads((out/'validation.json').read_text())
candidates={g['slug']:json.loads((out/g['slug']/'candidate.json').read_text()) for g in manifest['groups']}
entities={e['id']:e for d in candidates.values() for e in d['entities']};domain={t:g['title'] for g in manifest['groups'] for t in g['tables']}
def target(fk):return re.search(r'REFERENCES (?:public\.)?"?(\w+)"?',fk['definition'])[1]
def widen(e):
 badge=max([len(a['key']) if isinstance(a.get('key'),list) else int('key' in a) for a in e['attributes']]+[1]);keywidth=max(30,10+20*badge)
 e['width']=math.ceil(max(e.get('width',240),max(24+keywidth+6.6*len(a['name'])+6.3*len(a['type'])+18 for a in e['attributes'])))
for result in results:
 if result['ok']:continue
 slug=result['group'];d=candidates[slug];original=[e['id'] for e in d['entities']];closure=set(original)
 while True:
  more={target(fk) for n in closure for fk in tables[n]['constraints'] if fk['kind']=='f'}-closure
  if not more:break
  closure.update(more)
 added=sorted(closure-set(original),key=lambda n:(domain[n],n))
 if added:
  # All context tables retain their complete physical columns and real FK key roles.
  for i,e in enumerate(d['entities']):e['row']=i;e['col']=0
  grouped=collections.defaultdict(list)
  for name in added:grouped[domain[name]].append(name)
  col,row=1,0
  for tag,names in grouped.items():
   if row+len(names)>13:col+=1;row=0
   for name in names:
    e=copy.deepcopy(entities[name]);e['tag']='引用 · '+tag;e['row']=row;e['col']=col;row+=1;d['entities'].append(e)
  for name in original:
   for fk in tables[name]['constraints']:
    if fk['kind']!='f':continue
    t=target(fk)
    if t==name or any(x['id']==fk['name'] for x in d['relationships']):continue
    cols=fk['columns'];one=any(ix['unique'] and not ix['predicate'] and set(ix['columns'] or [])==set(cols) for ix in tables[name]['indexes'])
    d['relationships'].append(dict(id=fk['name'],**{'from':name,'to':t},label=', '.join(cols),fromCardinality='one' if one else 'many',toCardinality='one',fromOptional=True,toOptional=any(c['nullable'] for c in tables[name]['columns'] if c['name'] in cols)))
  d['cards'][0]['items'][0]=f'本域 {len(original)} 张表、{sum(len(tables[n]["columns"]) for n in original)} 个字段；另展开 {len(added)} 张外键目标表作为上下文，全部保留完整字段。'
  d['cards'][0]['items'][1]='引用域的表可在各自分图单独查看。连线展开本域的外键，引用域自身的外键保留在字段 references 和总览字典中。来自 615 份迁移在隔离 PostgreSQL 17 中重放后的最终 schema。'
 for e in d['entities']:widen(e)
 if not added and slug in ['channels','channel-delivery']:
  # Narrow the measured over-wide single row without hiding fields.
  d['layout']['gapX']=45
 (out/slug/'candidate.json').write_text(json.dumps(d,ensure_ascii=False,indent=2)+'\n')
 print(slug,len(original),'+',len(added),'context',flush=True)
