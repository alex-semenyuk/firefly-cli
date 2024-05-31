// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ethereum

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hyperledger/firefly-cli/internal/docker"
)

var entrypoint = "docker-entrypoint.sh"
var tmpath = "/qdata/tm"

func CreateTesseraKeys(ctx context.Context, image, outputDirectory, prefix, name, password string) (string, error) {
	// generates both .pub and .key files used by Tessera
	var filename string
	if prefix != "" {
		filename = fmt.Sprintf("%v_%s", prefix, name)
	} else {
		filename = name
	}
	if err := os.MkdirAll(outputDirectory, 0755); err != nil {
		return "", err
	}
	fmt.Println("generating tessera keys")
	err := docker.RunDockerCommand(ctx, outputDirectory, "run", "--rm", "-v", fmt.Sprintf("%s:/keystore", outputDirectory), image, "-keygen", "-filename", fmt.Sprintf("/keystore/%s", filename))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", outputDirectory, filename), nil
}

func CopyTesseraKeysToVolume(ctx context.Context, tesseraKeyPath, volumeName string) error {
	if err := docker.MkdirInVolume(ctx, volumeName, tmpath); err != nil {
		return err
	}
	if err := docker.CopyFileToVolume(ctx, volumeName, filepath.Join(tesseraKeyPath, "tm.pub"), tmpath); err != nil {
		return err
	}
	if err := docker.CopyFileToVolume(ctx, volumeName, filepath.Join(tesseraKeyPath, "tm.key"), tmpath); err != nil {
		return err
	}
	return nil
}

func CreateTesseraEntrypoint(ctx context.Context, outputDirectory, volumeName, memberCount string) error {
	// only tessera v09 onwards is supported
	var sb strings.Builder
	memberCountInt, _ := strconv.Atoi(memberCount)
	for i := 0; i < memberCountInt; i++ {
		sb.WriteString(fmt.Sprintf("{\"url\":\"http://member%dtessera:9000\"},", i)) // construct peer list
	}
	peerList := strings.TrimSuffix(sb.String(), ",")
	content := fmt.Sprintf(`export JAVA_OPTS="-Xms128M -Xmx128M"
DDIR=/data/qdata/dd
mkdir -p ${DDIR}
[ -f ${DDIR}/dd/tm.ipc ] && rm -f ${DDIR}/dd/tm.ipc
cat <<EOF > ${DDIR}/tessera-config-09.json
	{
		"useWhiteList": false,
		"jdbc": {
			"username": "sa",
			"password": "",
			"url": "jdbc:h2:./${DDIR}/db;TRACE_LEVEL_SYSTEM_OUT=0",
			"autoCreateTables": true
		},
		"serverConfigs":[
			{
				"app":"ThirdParty",
				"enabled": true,
				"serverAddress": "http://$(hostname -i):9080",
				"communicationType" : "REST"
			},
			{
				"app":"Q2T",
				"enabled": true,
				"serverAddress": "unix:${DDIR}/tm.ipc",
				"sslConfig": {
					"tls": "OFF"
				},
				"communicationType" : "REST"
			},
			{
				"app":"P2P",
				"enabled": true,
				"serverAddress": "http://$(hostname -i):9000",
				"sslConfig": {
					"tls": "OFF"
			},
				"communicationType" : "REST"
			}
		],
		"peer": [
			%s
		],
		"keys": {
		"passwords": [],
		"keyData": [
				{
					"privateKeyPath": "${DDIR}/keystore/tm.key",
					"publicKeyPath": "${DDIR}/keystore/tm.pub"
				}
			]
		},
		"alwaysSendTo": [],
		"bootstrapNode": false,
		"features": {
			"enableRemoteKeyValidation": false,
			"enablePrivacyEnhancements": true
		}
	}
EOF
/tessera/bin/tessera -configfile ${DDIR}/tessera-config-09.json
`, peerList)
	filename := filepath.Join(outputDirectory, entrypoint)
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	if err != nil {
		return err
	}
	CopyTesseraEntrypointToVolume(ctx, outputDirectory, volumeName)
	return nil
}

func CopyTesseraEntrypointToVolume(ctx context.Context, tesseraEntrypointDirectory, volumeName string) error {
	if err := docker.MkdirInVolume(ctx, volumeName, tmpath); err != nil {
		return err
	}
	if err := docker.CopyFileToVolume(ctx, volumeName, filepath.Join(tesseraEntrypointDirectory, entrypoint), tmpath); err != nil {
		return err
	}
	return nil
}
