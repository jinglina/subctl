/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

package broker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/rbac"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const InfoFileName = "broker-info.subm"

func WriteInfoToFile(restConfig *rest.Config, brokerNamespace string, ipsecPSK []byte, components sets.Set[string],
	customDomains []string, status reporter.Interface,
) error {
	status.Start("Saving broker info to file %q", InfoFileName)
	defer status.End()

	newFilename, err := backupIfExists(InfoFileName)
	if err != nil {
		return status.Error(err, "error backing up the broker file")
	}

	if newFilename != "" {
		status.Success("Backed up previous file %q to %q", InfoFileName, newFilename)
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return status.Error(err, "error creating Kubernetes client")
	}

	data := &Info{}

	data.ClientToken, err = rbac.GetClientTokenSecret(context.TODO(), kubeClient, brokerNamespace, constants.SubmarinerBrokerAdminSA)
	if err != nil {
		return errors.Wrap(err, "error getting broker client secret")
	}

	data.IPSecPSK = wrapIPSecPSKSecret(ipsecPSK)

	data.BrokerURL = restConfig.Host + restConfig.APIPath

	data.ServiceDiscovery = components.Has(component.ServiceDiscovery)
	data.Components = components.UnsortedList()
	sort.Strings(data.Components)

	if len(customDomains) > 0 {
		data.CustomDomains = &customDomains
	}

	return status.Error(data.writeToFile(InfoFileName), "error saving broker info")
}

func ReadInfoFromFile(filename string) (*Info, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading file %q", filename)
	}

	data := &Info{}

	bytes, err := base64.URLEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, errors.Wrapf(err, "error decoding data from file %q", filename)
	}

	return data, errors.Wrap(json.Unmarshal(bytes, data), "error unmarshalling data")
}

func backupIfExists(fileName string) (string, error) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return "", nil
	}

	now := time.Now()
	nowStr := strings.ReplaceAll(now.Format(time.RFC3339), ":", "_")
	newFilename := fileName + "." + nowStr

	return newFilename, os.Rename(fileName, newFilename) //nolint:wrapcheck // No need to wrap here
}
