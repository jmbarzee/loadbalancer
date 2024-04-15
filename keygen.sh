mkdir certs

openssl req -newkey rsa:2048 \
  -nodes -x509 \
  -days 30 \
  -out certs/ca.pem \
  -keyout certs/ca.key \
  -subj "/C=US/ST=California/L=San Francisco/O=ayada/OU=dev/CN=localhost"