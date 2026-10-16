import json,subprocess,pathlib,concurrent.futures
root=pathlib.Path.cwd();out=root/'.archify/erd-database-20261016-145515';manifest=json.loads((out/'atlas-manifest.json').read_text())
def run(g):
 p=out/g['slug'];html=p/(g['slug']+'.html')
 args=['node','/Users/qinqubo/.agents/skills/archify/bin/archify.mjs','finalize','erd',str(p/'candidate.json'),str(html),'--repo-root',str(root),'--quality','standard','--json']
 if html.exists():
  n=2
  while (p/f'review-{n}').exists():n+=1
  args+=['--out-dir',str(p/f'review-{n}')]
 r=subprocess.run(args,capture_output=True,text=True)
 (p/'finalize-output.json').write_text(r.stdout or r.stderr)
 try:d=json.loads(r.stdout)
 except:d={'ok':False,'diagnostics':[r.stderr]}
 print(json.dumps({'group':g['slug'],'ok':d.get('ok'),'stage':d.get('failedStage'),'diagnostics':[{k:x.get(k) for k in ['code','message','evidence']} if isinstance(x,dict) else x for x in d.get('diagnostics',[])][:4]},ensure_ascii=False),flush=True)
 return dict(group=g['slug'],ok=d.get('ok'),status=d.get('status'),receipt=d.get('evidence'),diagnostics=d.get('diagnostics',[]),visualReviewRecommendation=d.get('visualReviewRecommendation'))
previous=json.loads((out/'validation.json').read_text()) if (out/'validation.json').exists() else []
passed={x['group']:x for x in previous if x['ok']}
with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool: results=list(passed.values())+list(pool.map(run,[g for g in manifest['groups'] if g['slug'] not in passed]))
(out/'validation.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n')
print('TOTAL',len(results),'PASS',sum(bool(x['ok']) for x in results),flush=True)
