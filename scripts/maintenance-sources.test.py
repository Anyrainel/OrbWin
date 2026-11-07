import importlib.util
import pathlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("sources", pathlib.Path(__file__).with_name("collect-maintenance-sources.py"))
sources = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sources)


class SourceCollectionTests(unittest.TestCase):
    def test_offline_mode_cannot_make_a_network_request(self):
        previous = sources.OFFLINE
        sources.OFFLINE = True
        try:
            with patch.object(sources.urllib.request, 'urlopen', side_effect=AssertionError('network called')):
                with self.assertRaises(RuntimeError):
                    sources.fetch('https://example.invalid/source')
        finally:
            sources.OFFLINE = previous
    def test_literal_sources_aliases_and_checksum_binding(self):
        text = 'pkgname=probe\npkgver=1.2\nsource="renamed.tar::https://example.invalid/probe-$pkgver.tar.gz local.patch"\nsha512sums="\n' + 'a' * 128 + '  renamed.tar\n' + 'b' * 128 + '  local.patch\n"\n'
        variables, inputs = sources.recipe_inputs(text)
        self.assertEqual(variables['pkgver'], '1.2')
        self.assertEqual(inputs['renamed.tar']['url'], 'https://example.invalid/probe-1.2.tar.gz')
        self.assertNotIn('url', inputs['local.patch'])

    def test_recipes_cannot_execute_commands_or_guess_checksums(self):
        checksum = 'a' * 128 + '  source.tar'
        for url in ['https://example.invalid/$(touch-marker)/source.tar', 'https://example.invalid/$unknown/source.tar', 'http://example.invalid/source.tar', 'https://example.invalid/unreviewed.tar']:
            with self.subTest(url=url), self.assertRaises(ValueError):
                sources.recipe_inputs('source="' + url + '"\nsha512sums="' + checksum + '"')
        with self.assertRaises(ValueError):
            sources.recipe_inputs('sha512sums="SKIP  archive.tar"')
        with self.assertRaises(ValueError):
            sources.recipe_inputs('sha512sums="' + checksum + '\n' + checksum + '"')

    def test_paths_and_cached_bytes_fail_closed(self):
        with tempfile.TemporaryDirectory(prefix='orbwin-source-test-') as directory:
            base = pathlib.Path(directory).resolve()
            for name in ['../outside', '/outside', 'a/../b', 'a//b', 'a\\b', 'bad\0name']:
                with self.subTest(name=name), self.assertRaises(ValueError):
                    sources.local_path(base, name)
            path = base / 'source.txt'
            sources.publish(path, b'original')
            sources.publish(path, b'original')
            with self.assertRaises(ValueError):
                sources.publish(path, b'changed')
            self.assertEqual(path.read_bytes(), b'original')
            with self.assertRaises(ValueError):
                sources.read_bounded(path, 3)


if __name__ == '__main__':
    unittest.main()
