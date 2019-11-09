/*
Copyright 2019 The Tekton Authors

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

package webhook

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/tektoncd/triggers/pkg/interceptors"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"

	"go.uber.org/zap"
	"golang.org/x/xerrors"
	corev1 "k8s.io/api/core/v1"
)

// Timeout for outgoing requests to interceptor services
const interceptorTimeout = 5 * time.Second

type Interceptor struct {
	HTTPClient             *http.Client
	EventListenerNamespace string
	Logger                 *zap.SugaredLogger
	ObjectRef              *corev1.ObjectReference
}

func NewInterceptor(ei *triggersv1.EventInterceptor, c *http.Client, ns string, l *zap.SugaredLogger) interceptors.InterceptorInterface {
	return &Interceptor{
		HTTPClient:             c,
		EventListenerNamespace: ns,
		Logger:                 l,
		ObjectRef:              ei.Webhook.ObjectRef,
	}
}

func (w *Interceptor) ExecuteTrigger(payload []byte, request *http.Request, trigger triggersv1.EventListenerTrigger, eventID string) ([]byte, error) {
	interceptorURL, err := GetURI(w.ObjectRef, w.EventListenerNamespace) // TODO: Cache this result or do this on initialization
	if err != nil {
		return nil, err
	}

	modifiedPayload, err := w.processEvent(interceptorURL, request, payload, trigger.Interceptor.Header, interceptorTimeout)
	if err != nil {
		return nil, err
	}
	return modifiedPayload, nil
}

func (w *Interceptor) processEvent(interceptorURL *url.URL, request *http.Request, payload []byte, headerParams []pipelinev1.Param, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	outgoing := createOutgoingRequest(ctx, request, interceptorURL, payload)
	addInterceptorHeaders(outgoing.Header, headerParams)
	respPayload, err := makeRequest(w.HTTPClient, outgoing)
	if err != nil {
		return nil, xerrors.Errorf("Not OK response from Event Processor: %w", err)
	}
	return respPayload, nil
}

func addInterceptorHeaders(header http.Header, headerParams []pipelinev1.Param) {
	// This clobbers any matching headers
	for _, param := range headerParams {
		if param.Value.Type == pipelinev1.ParamTypeString {
			header[param.Name] = []string{param.Value.StringVal}
		} else {
			header[param.Name] = param.Value.ArrayVal
		}
	}
}
