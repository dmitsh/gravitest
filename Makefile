CLIENTS := client1 client2

.PHONY: build gen-proto gen-cert gen-cert-ca gen-cert-srv gen-cert-cln clean

build:
	go build ./cmd/server
	go build ./cmd/client
	go build ./cmd/runner

gen-proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/worker.proto

gen-cert: gen-cert-srv gen-cert-cln

gen-cert-ca:
	cd certs; openssl req -x509 -newkey rsa:4096 -keyout ca.key -out ca.crt -days 365 -nodes -subj "/CN=RootCA" -config host.conf

gen-cert-srv: gen-cert-ca
	cd certs; \
	openssl genrsa -out server.key 2048 && \
	openssl req -new -key server.key -subj "/CN=server" -out server.csr && \
	openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -extfile host-server.conf

gen-cert-cln: gen-cert-ca
	cd certs; \
	for cln in $(CLIENTS); do echo $$cln; \
	openssl genrsa -out $$cln.key 2048 && \
	openssl req -new -key $$cln.key -subj "/CN=$$cln" -out $$cln.csr && \
	openssl x509 -req -in $$cln.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out $$cln.crt -days 365; done

clean:
	rm -f server client runner
	cd certs; rm -f ca.* client1.* client2.* server.*
