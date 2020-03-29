
PROTOS_DIR = ./protobuf

.PHONY : all clean

all : protos

protos : $(PROTOS_DIR)
	python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. $(PROTOS_DIR)/*.proto

clean :
	@rm $(PROTOS_DIR)/*.py || true
	@rm $(PROTOS_DIR)/*.pyc || true
	@touch $(PROTOS_DIR)/__init__.py
