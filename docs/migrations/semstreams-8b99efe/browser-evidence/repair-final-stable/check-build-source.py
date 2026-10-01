import importlib.util,json
from pathlib import Path
root=Path('/Users/coby/.codex/worktrees/semstreams-frozen-migration/semteams');out=Path('/tmp/semteams-migration-8b99efe/repair/final-stable-build')
spec=importlib.util.spec_from_file_location('qualifier',root/'scripts/qualify-migration-browser.py');mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)
build=json.loads((out/'build-source-after.json').read_text());runtime=mod.source_snapshot(root)
changed=[name for name,digest in build.items() if not (root/name).is_file() or mod.sha(root/name)!=digest]
unlinked=[name for name,digest in runtime.items() if build.get(name)!=digest]
record={'build_sha256':mod.tree_sha(build),'runtime_sha256':mod.tree_sha(runtime),'build_file_count':len(build),'runtime_file_count':len(runtime),'changed_since_build':changed,'unlinked_runtime_files':unlinked,'valid':not changed and not unlinked}
mod.save(out/'browser-build-source-link.json',record);print(json.dumps(record,indent=2));raise SystemExit(0 if record['valid'] else 125)
