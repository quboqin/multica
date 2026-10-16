const fs=require('fs'); const path=require('path'); const {chromium}=require('playwright');
(async()=>{
 const dir=__dirname; const schema=JSON.parse(fs.readFileSync(path.join(dir,'schema.json')));const manifest=JSON.parse(fs.readFileSync(path.join(dir,'atlas-manifest.json')));const tableMap=new Map(schema.tables.map(t=>[t.name,t]));const owned=new Set();
 for(const g of manifest.groups){
  const d=JSON.parse(fs.readFileSync(path.join(dir,g.slug,'candidate.json')));
  if(!fs.existsSync(path.join(dir,g.slug,g.slug+'.html')))throw new Error('missing HTML '+g.slug);
  for(const name of g.tables){if(owned.has(name))throw new Error('duplicate owned table '+name);owned.add(name);const e=d.entities.find(e=>e.id===name);const t=tableMap.get(name);if(JSON.stringify(e.attributes.map(a=>a.name))!==JSON.stringify(t.columns.map(c=>c.name)))throw new Error('field coverage '+name);for(let i=0;i<t.columns.length;i++)if(!e.attributes[i].type.startsWith(t.columns[i].type))throw new Error('type '+name);}
 }
 if(owned.size!==tableMap.size)throw new Error('incomplete tables');
 const browser=await chromium.launch({channel:'chrome',headless:true});const page=await browser.newPage({viewport:{width:1440,height:1000}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('file://'+path.join(dir,'index.html'));await page.waitForSelector('#domains button');
 const navigation=await page.locator('#domains button').count();if(navigation!==31)throw new Error('navigation count '+navigation);
 await page.getByRole('button',{name:'字段字典',exact:true}).click();await page.locator('#table-workspace').waitFor({state:'visible'});
 await page.locator('#search').fill('runtime_mcp_overlay');await page.locator('#results button[data-table="agent_task_queue"]').click();await page.locator('#table-agent_task_queue').waitFor({state:'visible'});
 const fieldCount=await page.locator('#table-agent_task_queue tbody tr').count();if(fieldCount!==59)throw new Error('run fields '+fieldCount);
 await page.locator('#search').fill('');await page.locator('[data-group="collections"]').click();await page.getByRole('button',{name:'ER 图',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('#frame').contentWindow!==null);await page.waitForTimeout(400);
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);if(overflow)throw new Error('index overflow');
 await page.setViewportSize({width:390,height:844});await page.waitForTimeout(100);const mobileOverflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);if(mobileOverflow)throw new Error('mobile index overflow');
 await browser.close();if(errors.length)throw new Error(errors.join('\n'));
 const result={ok:true,tables:owned.size,columns:schema.tables.reduce((a,t)=>a+t.columns.length,0),domains:navigation,queueFields:fieldCount,desktopOverflow:overflow,mobileOverflow,pageErrors:errors};fs.writeFileSync(path.join(dir,'atlas-check.json'),JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));
})().catch(e=>{console.error(e);process.exit(1)});
