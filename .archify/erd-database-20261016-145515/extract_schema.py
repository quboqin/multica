import json,os,subprocess,tempfile,pathlib
root=pathlib.Path.cwd(); out=root/'.archify/erd-database-20261016-145515'
tmp=pathlib.Path(tempfile.mkdtemp(prefix='multica-erd-')); data=tmp/'data'; log=out/'schema-replay.log'
def run(args,**kw):
 return subprocess.run(args,check=True,text=True,**kw)
query="""
SELECT jsonb_build_object('tables',COALESCE(jsonb_agg(t ORDER BY t->>'name'),'[]'::jsonb)) FROM (
 SELECT jsonb_build_object('name',c.relname,'kind',c.relkind,'comment',obj_description(c.oid,'pg_class'),
 'columns',(SELECT jsonb_agg(jsonb_build_object('name',a.attname,'type',format_type(a.atttypid,a.atttypmod),'nullable',NOT a.attnotnull,'default',pg_get_expr(ad.adbin,ad.adrelid),'comment',col_description(c.oid,a.attnum),'position',a.attnum) ORDER BY a.attnum) FROM pg_attribute a LEFT JOIN pg_attrdef ad ON ad.adrelid=a.attrelid AND ad.adnum=a.attnum WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),
 'constraints',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',co.conname,'kind',co.contype,'definition',pg_get_constraintdef(co.oid),'columns',(SELECT jsonb_agg(a.attname ORDER BY u.ord) FROM unnest(co.conkey) WITH ORDINALITY u(num,ord) JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=u.num),'target',CASE WHEN co.confrelid>0 THEN co.confrelid::regclass::text ELSE NULL END)) FROM pg_constraint co WHERE co.conrelid=c.oid),'[]'::jsonb),
 'indexes',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',ic.relname,'unique',i.indisunique,'primary',i.indisprimary,'valid',i.indisvalid,'predicate',pg_get_expr(i.indpred,i.indrelid),'definition',pg_get_indexdef(i.indexrelid),'columns',(SELECT jsonb_agg(a.attname ORDER BY u.ord) FROM unnest(i.indkey) WITH ORDINALITY u(num,ord) JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=u.num WHERE u.ord<=i.indnkeyatts))) FROM pg_index i JOIN pg_class ic ON ic.oid=i.indexrelid WHERE i.indrelid=c.oid),'[]'::jsonb)) AS t
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p')
) q;
"""
started=False
container='multica-erd-20261016-145515'
try:
 with log.open('w') as f:
  run(['docker','run','--rm','-d','--name',container,'-e','POSTGRES_USER=archify','-e','POSTGRES_HOST_AUTH_METHOD=trust','-p','127.0.0.1::5432','pgvector/pgvector:pg17'],stdout=f,stderr=f);started=True
  import time
  for _ in range(50):
   check=subprocess.run(['docker','exec',container,'pg_isready','-U','archify','-d','postgres'],capture_output=True)
   if check.returncode==0:break
   time.sleep(.2)
  else:raise RuntimeError('ephemeral PostgreSQL not ready')
  binding=run(['docker','port',container,'5432/tcp'],capture_output=True).stdout.strip()
  assert binding.startswith('127.0.0.1:')
  port=binding.split(':')[-1]
  env=os.environ.copy();env['DATABASE_URL']=f'postgresql://archify@127.0.0.1:{port}/postgres?sslmode=disable'
  run(['go','build','-o',str(tmp/'migrate'),'./cmd/migrate'],cwd=root/'server',stdout=f,stderr=f)
  run([str(tmp/'migrate'),'up'],cwd=root/'server',env=env,stdout=f,stderr=f)
  psql=['docker','exec',container,'psql','-U','archify','-d','postgres','-X','-A','-t','-v','ON_ERROR_STOP=1']
  result=run(psql+['-c',query],capture_output=True)
  schema=json.loads(result.stdout)
  schema['revision']=run(['git','rev-parse','HEAD'],capture_output=True).stdout.strip()
  schema['postgres']=run(psql+['-c','SELECT version();'],capture_output=True).stdout.strip()
  schema['migrations']=run(psql+['-c','SELECT count(*) FROM schema_migrations;'],capture_output=True).stdout.strip()
  (out/'schema.json').write_text(json.dumps(schema,ensure_ascii=False,indent=2)+'\n')
  with (out/'schema.sql').open('w') as dump:
   run(['docker','exec',container,'pg_dump','-U','archify','-d','postgres','--schema-only','--no-owner','--no-privileges'],stdout=dump,stderr=f)
  print(json.dumps({'tables':len(schema['tables']),'columns':sum(len(x['columns']) for x in schema['tables']),'foreign_keys':sum(sum(c['kind']=='f' for c in x['constraints']) for x in schema['tables']),'migrations':schema['migrations']}))
finally:
 if started:run(['docker','stop',container],stdout=subprocess.DEVNULL)
 import shutil
 shutil.rmtree(tmp)
