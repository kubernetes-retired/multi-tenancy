#!/usr/bin/env bash

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# gencerts.sh generates the certificates for the generic TLS test.

TENANT_NAME=tenantA

cat > server.conf << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
EOF

# Create a certificate authority
openssl genrsa -out CAKey.pem 2048
openssl req -x509 -new -nodes -key CAKey.pem -days 100000 -out CACert.pem -subj "/CN=root_ca"

# Create a kubelet server certiticate
openssl genrsa -out KubeletServerKey.pem 2048
openssl req -new -key KubeletServerKey.pem -out KubeletServer.csr -subj "/CN=kubelet_server" -config server.conf
openssl x509 -req -in KubeletServer.csr -CA CACert.pem -CAkey CAKey.pem -CAcreateserial -out KubeletServerCert.pem -days 100000 -extensions v3_req -extfile server.conf

# Create a kubelet client certiticate
openssl genrsa -out KubeletClientKey.pem 2048
openssl req -new -key KubeletClientKey.pem -out KubeletClient.csr -subj "/CN=kubelet_client" -config server.conf
openssl x509 -req -in KubeletClient.csr -CA CACert.pem -CAkey CAKey.pem -CAcreateserial -out KubeletClientCert.pem -days 100000 -extensions v3_req -extfile server.conf

# Create a vn agent certiticate
openssl genrsa -out VnAgentKey.pem 2048
openssl req -new -key VnAgentKey.pem -out VnAgent.csr -subj "/CN=vnagent_server" -config server.conf
openssl x509 -req -in VnAgent.csr -CA CACert.pem -CAkey CAKey.pem -CAcreateserial -out VnAgentCert.pem -days 100000 -extensions v3_req -extfile server.conf

# Create a tenant certiticate
openssl genrsa -out TenantKey.pem 2048
openssl req -new -key TenantKey.pem -out Tenant.csr -subj "/CN=${TENANT_NAME}" -config server.conf
openssl x509 -req -in Tenant.csr -CA CACert.pem -CAkey CAKey.pem -CAcreateserial -out TenantCert.pem -days 100000 -extensions v3_req -extfile server.conf

outfile=certs.go

cat > $outfile << EOF
/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file was generated using openssl by the gencerts.sh script
// and holds raw certificates for the webhook tests.

package testcerts

EOF

echo "// TenantName is the CommonName in the tenant cert." >> $outfile
echo "const TenantName = \"${TENANT_NAME}\"" >> $outfile

for file in CA KubeletServer KubeletClient VnAgent Tenant; do
  for type in Key Cert; do
    data=$(cat "${file}${type}".pem)
    echo "" >> $outfile
    echo "var ${file}${type} = []byte(\`$data\`)" >> $outfile
  done
done

# Clean up after we're done.
rm ./*.pem
rm ./*.csr
rm ./*.srl
rm ./*.conf
