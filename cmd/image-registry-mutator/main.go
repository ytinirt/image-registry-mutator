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
	"fmt"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	tlsDir      = `/run/secrets/tls`
	tlsCertFile = `tls.crt`
	tlsKeyFile  = `tls.key`
)

var (
	podResource = metav1.GroupVersionResource{Version: "v1", Resource: "pods"}
)

func needMutating(pod *corev1.Pod) bool {
	ret := false

	for _, c := range pod.Spec.Containers {
		if !strings.Contains(c.Image, ":") {
			log.Printf("container[%s]'s image[%s] mutating to tag latest", c.Name, c.Image)
			ret = true
		} else {
			log.Printf("container[%s]'s image[%s]", c.Name, c.Image)
		}
	}

	return ret
}

func generatePatch(pod *corev1.Pod) ([]patchOperation, error) {
	var patches []patchOperation

	for i, c := range pod.Spec.Containers {
		if strings.Contains(c.Image, ":") {
			continue
		}

		patches = append(patches, patchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/containers/%d/image", i),
			Value: fmt.Sprintf("%s:latest", c.Image),
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

func main() {
	certPath := filepath.Join(tlsDir, tlsCertFile)
	keyPath := filepath.Join(tlsDir, tlsKeyFile)

	mux := http.NewServeMux()
	mux.Handle("/mutate", admitFuncHandler(mutateImageRegistry))
	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServeTLS(certPath, keyPath))
}
