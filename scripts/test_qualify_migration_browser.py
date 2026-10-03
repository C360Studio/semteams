"""Keep browser qualification sensitive to changes in product-owned Go packages."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

MODULE = importlib.util.spec_from_file_location(
    "qualify_migration_browser", Path(__file__).with_name("qualify-migration-browser.py")
)
runner = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(runner)


class SourceSnapshotTest(unittest.TestCase):
    def test_internal_runtime_change_invalidates_source_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            for name in ("go.mod", "go.sum", "ui/package-lock.json", "internal/runtimecatalog/register.go"):
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("before\n")
            before = runner.source_snapshot(source)
            (source / "internal/runtimecatalog/register.go").write_text("after\n")
            after = runner.source_snapshot(source)
            self.assertNotEqual(runner.tree_sha(before), runner.tree_sha(after))
            self.assertIn("internal/runtimecatalog/register.go", before)


if __name__ == "__main__":
    unittest.main()
