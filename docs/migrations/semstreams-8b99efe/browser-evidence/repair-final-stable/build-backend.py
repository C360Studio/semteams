import hashlib,importlib.util,json,subprocess,time
from pathlib import Path
root=Path('/Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams')
out=Path('/tmp/semteams-migration-8b99efe/repair/final-stable-build')
spec=importlib.util.spec_from_file_location('qualifier',root/'scripts/qualify-migration-browser.py');mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)
def snapshot():
 files=mod.source_snapshot(root)
 for path in [root/'docker/Dockerfile',root/'.dockerignore',*(root/'.devcontainer').rglob('*')]:
  if path.is_file():files[str(path.relative_to(root))]=mod.sha(path)
 return dict(sorted(files.items()))
before=snapshot();mod.save(out/'build-source-before.json',before)
cmd=['docker','compose','-p','stmig162','-f',str(out/'compose.json'),'build','backend']
with (out/'backend-build.log').open('w') as log:
 result=subprocess.run(cmd,cwd=root,stdout=log,stderr=subprocess.STDOUT)
after=snapshot();mod.save(out/'build-source-after.json',after)
record={'command':cmd,'exit':result.returncode,'before_sha256':mod.tree_sha(before),'after_sha256':mod.tree_sha(after),'unchanged':before==after,'completed_at':time.time(),'git_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()}
if result.returncode==0:
 record['image']=json.loads(subprocess.check_output(['docker','image','inspect','stmig162-backend:autoresearch-repair-stable'],text=True))[0]
mod.save(out/'backend-build.json',record)
print(json.dumps({k:v for k,v in record.items() if k!='image'},indent=2))
raise SystemExit(result.returncode if before==after else 125)
