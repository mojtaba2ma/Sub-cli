import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import urllib.parse, json, sys

LINKS = """vless://11111111-1111-4111-8111-111111111111@cdn.example.com:443?type=ws&security=tls&encryption=none&sni=cdn.example.com&fp=chrome&alpn=h2%2Chttp%2F1.1&host=cdn.example.com&path=%2Fmyws%3Fed%3D2560&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F8.8.8.8%2Fdns-query#%D9%86%D8%A7%D9%85-%D8%A7%D9%88%D9%84
vless://22222222-2222-4222-8222-222222222222@1.2.3.4:8443?type=ws&security=tls&sni=sni.example.com&host=sni.example.com&path=%2Fws#second-node
vmess://eyJ2IjoiMiIsInBzIjoiTm9kZS1WbWVzcyIsImFkZCI6ImNkbi5leGFtcGxlLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjMzMzMzMzMzLTMzMzMtNDMzMy04MzMzLTMzMzMzMzMzMzMzMyIsImFpZCI6IjAiLCJzY3kiOiJhdXRvIiwibmV0Ijoid3MiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJjZG4uZXhhbXBsZS5jb20iLCJwYXRoIjoiL3dtZXNzIiwidGxzIjoidGxzIiwic25pIjoiY2RuLmV4YW1wbGUuY29tIn0=
"""
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path.startswith("/sub/"):
            body = base64.b64encode(LINKS.encode()).decode().encode()
            self.send_response(200)
            self.send_header("Content-Type","text/plain")
            self.send_header("Subscription-Userinfo","upload=1234; download=5678; total=107374182400; expire=1790000000")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body)
        else:
            self.send_response(404); self.end_headers()
ThreadingHTTPServer(("127.0.0.1", 2096), H).serve_forever()
