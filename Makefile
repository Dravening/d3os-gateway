build:
	docker build -t registry.cn-hangzhou.aliyuncs.com/draven_yyz/d3os-gateway:v1.0 .

push:
	docker push registry.cn-hangzhou.aliyuncs.com/draven_yyz/d3os-gateway:v1.0

total: build push