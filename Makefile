
CC = go build

PROD = Low-Latency-FaaS
HANDLERS_DIR = ./handlers
HANDLERS = $(wildcard $(HANDLERS_DIR)/*.go)
CONTROLLER_DIR = ./controller
CONTROLLER = $(wildcard $(CONTROLLER_DIR)/*.go)
PROTOS_DIR = ./proto

.PHONY : all clean fmt

all : fmt protos $(PROD)

fmt :
	@gofmt -l -s -w .

tests :
	@go test -v ./...

protos : $(PROTOS_DIR)
	protoc -I $(PROTOS_DIR) --go_out=plugins=grpc:$(PROTOS_DIR) $(PROTOS_DIR)/*.proto

$(PROD) : main.go $(HANDLERS) $(CONTROLLER)
	$(CC) -o $(PROD) .

clean :
	@rm $(PROTOS_DIR)/*.pb.go
	@rm $(PROD) || true
