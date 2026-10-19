"""Black-box tests against the actual built transfer.sh server; stdlib only."""
import json
import os
import unittest
import urllib.error
import urllib.parse
import urllib.request

BASE = os.environ.get('TEST_BASE_URL', 'http://127.0.0.1:8080').rstrip('/')
SAMPLE = b'192.0.2.10:8080\n198.51.100.25:3128\n203.0.113.50:8080\n'


def request(path, data=None, method='GET', headers=None):
    headers = {'ngrok-skip-browser-warning': 'integration-test', **(headers or {})}
    url = path if path.startswith(('http://', 'https://')) else BASE + path
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.headers, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read()


def upload(name, body=SAMPLE, mime='text/plain', extra=None):
    status, headers, response = request('/' + urllib.parse.quote(name), body, 'PUT', {'Content-Type': mime, **(extra or {})})
    assert status == 200, (status, response)
    raw = response.decode().strip()
    parsed = urllib.parse.urlsplit(raw)
    assert parsed.scheme in ('http', 'https') and parsed.netloc and not parsed.query and not parsed.fragment, raw
    parts = parsed.path.split('/')
    assert len(parts) == 3 and parts[1] and parts[2], raw
    assert '\n' not in raw and '<' not in raw, raw
    assert headers['Content-Type'].startswith('text/plain')
    assert urllib.parse.urlsplit(headers['X-View-URL']).path == '/view' + parsed.path
    assert urllib.parse.urlsplit(headers['X-Direct-URL']).path == '/get' + parsed.path
    return parsed.path


class TransferTests(unittest.TestCase):
    def test_upload_preview_and_original(self):
        path = upload('http_proxies.txt')
        status, h, body = request('/view' + path)
        self.assertEqual(status, 200)
        self.assertTrue(h['Content-Type'].startswith('text/html'))
        self.assertIn(b'http_proxies.txt', body)
        self.assertIn(SAMPLE.strip(), body)
        self.assertIn(('/get' + path).encode(), body)
        self.assertIn(b'Uploaded', body)
        self.assertNotIn(b'Expires</dt>', body)
        for prefix in ('/get', '', '/inline'):
            status, headers, data = request(prefix + path, headers={'Accept': 'text/html'})
            self.assertEqual(status, 200)
            self.assertEqual(data, SAMPLE)
            if prefix == '/get':
                self.assertIn('attachment;', headers['Content-Disposition'])
                self.assertIn('http_proxies.txt', headers['Content-Disposition'])
        status, headers, _ = request('/get' + path, method='HEAD')
        self.assertEqual(status, 200)
        self.assertEqual(int(headers['Content-Length']), len(SAMPLE))

    def test_preview_limit_preserves_full_download(self):
        data = b'x' * (256 * 1024) + b'END-OF-FULL-FILE'
        path = upload('large.txt', data)
        status, _, preview = request('/view' + path)
        self.assertEqual(status, 200)
        self.assertIn(b'Preview limited', preview)
        self.assertNotIn(b'END-OF-FULL-FILE', preview)
        self.assertEqual(request('/get' + path)[2], data)

    def test_source_json_csv_and_binary(self):
        for filename in ('active.html', 'active.svg'):
            path = upload(filename, b'<script>window.evil=true</script>', 'text/html')
            page = request('/view' + path)[2]
            self.assertIn(b'&lt;script&gt;', page)
            self.assertNotIn(b'<script>window.evil', page)
            _, headers, source = request('/inline' + path)
            self.assertTrue(headers['Content-Type'].startswith('text/plain'))
            self.assertIn('sandbox', headers['Content-Security-Policy'])
            self.assertEqual(source, b'<script>window.evil=true</script>')
        path = upload('data.json', b'{"answer":42}', 'application/json')
        self.assertIn(b'  &#34;answer&#34;: 42', request('/view' + path)[2])
        path = upload('data.csv', b'name,value\nalpha,"two,three"\n', 'text/csv')
        self.assertIn(b'<td>two,three</td>', request('/view' + path)[2])
        path = upload('archive.zip', b'PK\x03\x04\x00\xff', 'application/zip')
        self.assertIn(b'An inline preview is unavailable', request('/view' + path)[2])
        self.assertEqual(request('/get' + path)[2], b'PK\x03\x04\x00\xff')

    def test_unicode_and_url_characters(self):
        path = upload('a #?&é.txt')
        self.assertEqual(request('/view' + path)[0], 200)
        status, headers, body = request('/get' + path)
        self.assertEqual((status, body), (200, SAMPLE))
        self.assertIn('attachment', headers['Content-Disposition'])

    def test_expired_missing_and_deleted(self):
        self.assertEqual(request('/view/notfound/missing.txt')[0], 404)
        self.assertEqual(request('/get/notfound/missing.txt')[0], 404)
        self.assertEqual(request('/inline/notfound/missing.txt')[0], 404)
        # Max-Days 0 expires immediately and is rejected by the existing upload handler.
        status, _, _ = request('/expired.txt', SAMPLE, 'PUT', {'Max-Days': '-1', 'Content-Type': 'text/plain'})
        self.assertNotEqual(status, 200)
        path = upload('limit.txt', extra={'Max-Downloads': '1'})
        self.assertEqual(request('/view' + path)[0], 200)
        self.assertEqual(request('/get' + path)[0], 200)
        self.assertEqual(request('/view' + path)[0], 404)
        status, headers, _ = request('/delete.txt', SAMPLE, 'PUT', {'Content-Type': 'text/plain'})
        self.assertEqual(status, 200)
        deletion = urllib.parse.urlsplit(headers['X-Url-Delete']).path
        self.assertEqual(request(deletion, method='DELETE')[0], 200)
        self.assertEqual(request('/view/' + '/'.join(deletion.split('/')[1:3]))[0], 404)

    def test_multipart_compatibility(self):
        boundary = 'transfer-integration-boundary'
        data = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="form.txt"\r\nContent-Type: text/plain\r\n\r\n'.encode() + SAMPLE + f'\r\n--{boundary}--\r\n'.encode())
        status, h, body = request('/', data, 'POST', {'Content-Type': f'multipart/form-data; boundary={boundary}'})
        self.assertEqual(status, 200)
        path = urllib.parse.urlsplit(body.decode().strip()).path
        self.assertIn('/view' + path, h['X-View-URL'])
        self.assertEqual(request('/get' + path)[2], SAMPLE)


if __name__ == '__main__':
    unittest.main(verbosity=2)
