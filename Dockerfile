FROM golang:1.25 AS builder
LABEL authors="da.wells"

ARG cert_location=/usr/local/share/ca-certificates

# Get certificate from "github.com"
RUN openssl s_client -showcerts -connect github.com:443 </dev/null 2>/dev/null|openssl x509 -outform PEM > ${cert_location}/github.crt
# Get certificate from "proxy.golang.org"
RUN openssl s_client -showcerts -connect proxy.golang.org:443 </dev/null 2>/dev/null|openssl x509 -outform PEM >  ${cert_location}/proxy.golang.crt
# Update certificates
RUN update-ca-certificates

COPY . .
RUN git config --global http.sslVerify false
RUN go env -w GOPROXY=direct GOFLAGS="-insecure"
RUN go mod download github.com/gorilla/mux
RUN go build main.go

EXPOSE 8000

CMD ["./main"]