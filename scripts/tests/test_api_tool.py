"""Exercise the real official checker with incompatible/compatible Go changes."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

class RealAPIToolTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tool=os.environ.get('APIDIFF') or shutil.which('apidiff') or str(Path(__file__).parents[2]/'.cache/sdk-tools/apidiff')
        if not Path(cls.tool).is_file():raise unittest.SkipTest('Install pinned apidiff; compatibility CI executes these real-tool tests')
    def command(self,folder,*args):
        result=subprocess.run([self.tool,*args],cwd=folder,env={**os.environ,'GOWORK':'off'},capture_output=True,text=True)
        self.assertEqual(result.returncode,0,result.stderr)
        return result.stdout
    def test_changed_interface_constant_and_removed_field_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            folder=Path(directory)
            (folder/'go.mod').write_text('module example.com/contractfixture\n\ngo 1.26.0\n')
            source=folder/'api.go'
            source.write_text('package fixture\n type Store interface { Get(string) error }; type Options struct { Limit int }; const Code="STABLE"\n')
            old,new=str(folder/'old.api'),str(folder/'new.api')
            self.command(folder,'-m','-w',old,'example.com/contractfixture')
            source.write_text('package fixture\n type Store interface { Get(int) error }; type Options struct {}; const Code="CHANGED"\n')
            self.command(folder,'-m','-w',new,'example.com/contractfixture')
            report=self.command(folder,'-m','-incompatible',old,new)
            for name in ('Store.Get','Options.Limit','Code'):self.assertIn(name,report)
    def test_additive_change_requires_minor_review(self):
        with tempfile.TemporaryDirectory() as directory:
            folder=Path(directory);(folder/'go.mod').write_text('module example.com/addition\n\ngo 1.26.0\n')
            source=folder/'api.go';source.write_text('package fixture\nfunc Existing() {}\n')
            old,new=str(folder/'old.api'),str(folder/'new.api')
            self.command(folder,'-m','-w',old,'example.com/addition')
            source.write_text('package fixture\nfunc Existing() {}\nfunc Added() {}\n')
            self.command(folder,'-m','-w',new,'example.com/addition')
            self.assertEqual(self.command(folder,'-m','-incompatible',old,new),'')
            self.assertIn('Compatible changes:',self.command(folder,'-m',old,new))

if __name__=='__main__':unittest.main()
