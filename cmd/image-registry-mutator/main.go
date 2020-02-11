/*
Copyright (c) 2019      StackRox Inc.
Copyright (c) 2019-2020 ZHAO Yao <ytinirt@qq.com>

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

package main

import (
	"encoding/csv"
	"fmt"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	tlsDir      	= `/run/secrets/tls`
	tlsCertFile 	= `tls.crt`
	tlsKeyFile  	= `tls.key`

	envKeyBypassMe	= `IRM_BYPASS_ME`	// If present, bypass image registry mutator's pod itself
	envKeyBypassNS	= `IRM_BYPASS_NS`	// If present, bypass all namespaces, CSV formatted
	envKeyRegistry	= `IRM_REGISTRY`	// If present, replace registry with it, otherwise do nothing
)

var (
	podResource = metav1.GroupVersionResource{Version: "v1", Resource: "pods"}

	bypassNS map[string]string = map[string]string {}
	bypassMe bool = false
	registry string = ""				// not end with '/'
)

func needMutating(pod *corev1.Pod) (ret bool) {
	ret = false

	if registry == "" {
		log.Printf("registry not defined, give up mutating")
		return
	}

	if _, bypass := bypassNS[pod.Namespace]; bypass {
		log.Printf("bypass namespace %s", pod.Namespace)
		return
	}

	var name string
	if pod.Name == "" {
		name = pod.GenerateName
	} else {
		name = pod.Name
	}

	if bypassMe && pod.Namespace == "kube-system" && strings.HasPrefix(name, "image-registry-mutator-") {
		// FIXME: hard-code
		log.Printf("bypass myself kube-system/image-registry-mutator-*")
		return
	}

	for _, c := range pod.Spec.Containers {
		if !strings.HasPrefix(c.Image, fmt.Sprintf("%s/", registry)) {
			log.Printf("container[%s]'s image[%s] not from registry %s, mutating it", c.Name, c.Image, registry)
			ret = true
		} else {
			log.Printf("container[%s]'s image[%s], do not need mutating", c.Name, c.Image)
		}
	}

	return
}

func generatePatch(pod *corev1.Pod) ([]patchOperation, error) {
	var patches []patchOperation

	for i, c := range pod.Spec.Containers {
		if strings.HasPrefix(c.Image, fmt.Sprintf("%s/", registry)) {
			continue
		}

		patches = append(patches, patchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/containers/%d/image", i),
			Value: fmt.Sprintf("%s/%s", registry, c.Image),
		})
	}

	return patches, nil
}

func mutateImageRegistry(req *v1beta1.AdmissionRequest) ([]patchOperation, error) {
	// This handler should only get called on Pod objects as per the MutatingWebhookConfiguration in the YAML file.
	// However, if (for whatever reason) this gets invoked on an object of a different kind, issue a log message but
	// let the object request pass through otherwise.
	if req.Resource != podResource {
		log.Printf("expect resource to be %s, rather than %s", podResource, req.Resource)
		return nil, nil
	}

	// Parse the Pod object.
	raw := req.Object.Raw
	pod := corev1.Pod{}
	if _, _, err := universalDeserializer.Decode(raw, nil, &pod); err != nil {
		return nil, fmt.Errorf("could not deserialize pod object: %v", err)
	}

	if pod.Namespace == "" {
		pod.Namespace = req.Namespace
		log.Printf("populate pod's namespace with admission request's namespace %s", req.Namespace)
	}

	if !needMutating(&pod) {
		log.Printf("pod %s/%s do not need mutating", pod.Namespace, pod.Name)
		return nil, nil
	}

	patches, err := generatePatch(&pod)
	if err != nil {
		return nil, fmt.Errorf("generate patch failed")
	}
	log.Printf("patches: %v", patches)
	return patches, nil
}

func initConfig() {
	var val string

	// bypassMe
	if _, present := os.LookupEnv(envKeyBypassMe); present {
		log.Printf("Env %s is present, bypass me", envKeyBypassMe)
		bypassMe = true
	} else {
		log.Printf("Missing env %s", envKeyBypassMe)
	}

	// registry
	val = os.Getenv(envKeyRegistry)
	if val != "" {
		registry = strings.TrimRight(val, "/")
		log.Printf("Env %s is setted, value %s, registry %s", envKeyRegistry, val, registry)
	} else {
		log.Printf("Missing env %s or value is empty", envKeyRegistry)
	}

	// bypassNS
	val = os.Getenv(envKeyBypassNS)
	if val != "" {
		r := csv.NewReader(strings.NewReader(val))
		records, err := r.ReadAll()
		if err == nil {
			var flatRecords []string
			for _, record := range records {
				flatRecords = append(flatRecords, record...)
			}
			log.Printf("Env %s is setted, value %s, CSV parsed %v", envKeyBypassNS, val, flatRecords)
			for _, ns := range flatRecords {
				bypassNS[ns] = ""
			}
		} else {
			log.Printf("Env %s is setted, value %s, CSV parse failed with error: %v", envKeyBypassNS, val, err)
		}
	} else {
		log.Printf("Missing env %s or value is empty", envKeyBypassNS)
	}

	return
}

func main() {
	certPath := filepath.Join(tlsDir, tlsCertFile)
	keyPath := filepath.Join(tlsDir, tlsKeyFile)

	initConfig()

	mux := http.NewServeMux()
	mux.Handle("/mutate", admitFuncHandler(mutateImageRegistry))
	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServeTLS(certPath, keyPath))
}
