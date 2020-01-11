# Copyright (c) 2019      StackRox Inc.
# Copyright (c) 2019-2020 ZHAO Yao <ytinirt@qq.com>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.DEFAULT_GOAL := docker-image

IMAGE ?= ytinirt/image-registry-mutator:latest

image/image-registry-mutator: $(shell find . -name '*.go')
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $@ ./cmd/image-registry-mutator

.PHONY: docker-image
docker-image: image/image-registry-mutator
	sudo docker build -t $(IMAGE) image/

.PHONY: push-image
push-image: docker-image
	sudo docker push $(IMAGE)
