#!/usr/bin/env bash

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

# deploy.sh
#
# Deploy image-registry-mutator in an active cluster.

DEPLOY_NAMESPACE=${1:-kube-system}

set -euo pipefail

basedir="$(dirname "$0")/deployment"
keydir="$(mktemp -d)"

# Generate keys into a temporary directory.
echo "Generating TLS keys ..."
# substitute ${DEPLOY_NAMESPACE} in ${basedir}/generate-keys.sh.template
sed -e 's@${DEPLOY_NAMESPACE}@'"$DEPLOY_NAMESPACE"'@g' <"${basedir}/generate-keys.sh.template" \
    >"${basedir}/generate-keys.sh"
sh "${basedir}/generate-keys.sh" "$keydir"

# Create the TLS secret for the generated keys.
kubectl -n ${DEPLOY_NAMESPACE} create secret tls image-registry-mutator-tls \
    --cert "${keydir}/image-registry-mutator-tls.crt" \
    --key "${keydir}/image-registry-mutator-tls.key"

# Read the PEM-encoded CA certificate, base64 encode it, and replace the `${CA_PEM_B64}` placeholder in the YAML
# template with it. Then, create the Kubernetes resources.
ca_pem_b64="$(openssl base64 -A <"${keydir}/ca.crt")"
sed -e 's@${CA_PEM_B64}@'"$ca_pem_b64"'@g' <"${basedir}/deployment.yaml.template" \
    | sed -e 's@${DEPLOY_NAMESPACE}@'"$DEPLOY_NAMESPACE"'@g' \
    >"${basedir}/deployment.yaml"
kubectl -n ${DEPLOY_NAMESPACE} create -f "${basedir}/deployment.yaml"

# Delete the key directory to prevent abuse (DO NOT USE THESE KEYS ANYWHERE ELSE).
rm -rf "$keydir"

echo "The image registry mutator has been deployed and configured!"
