# /etc/nginx/sites-available/documents.atmosferast.com (symlinked from sites-enabled)
server {
	listen 80;
	listen [::]:80;
	server_name documents.atmosferast.com;
	return 301 https://$host$request_uri;
}

server {
	listen 443 ssl http2;
	listen [::]:443 ssl http2;
	server_name documents.atmosferast.com;

	# Wildcard certificate shared with the other *.atmosferast.com sites.
	ssl_certificate     /etc/ssl/atmosferast/origin.pem;
	ssl_certificate_key /etc/ssl/atmosferast/origin.key;

	# The API accepts bodies up to 4 MiB (a 2 MiB logo grows by a third in base64).
	client_max_body_size 5m;

	location / {
		proxy_pass http://127.0.0.1:3133;
		proxy_http_version 1.1;
		proxy_set_header Host $host;
		proxy_set_header X-Real-IP $remote_addr;
		proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
		proxy_set_header X-Forwarded-Proto $scheme;
		# The API caps a request at RENDER_TIMEOUT (15 s) and writes within 30 s.
		proxy_read_timeout 35s;
		proxy_send_timeout 35s;
	}
}
