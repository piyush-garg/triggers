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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
)

func Test_processEvent(t *testing.T) {
	i := Interceptor{HTTPClient: http.DefaultClient}

	payload, _ := json.Marshal(map[string]string{
		"eventType": "push",
		"foo":       "bar",
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cmp.Diff(r.Header["Param-Header"], []string{"val"}) != "" {
			http.Error(w, "Expected header does not match", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer ts.Close()

	incoming := httptest.NewRequest("POST", "http://event.listener.url", nil)
	incoming.Header.Add("Content-type", "application/json")

	interceptorURL, _ := url.Parse(ts.URL)
	params := []pipelinev1.Param{{
		Name: "Param-Header",
		Value: pipelinev1.ArrayOrString{
			Type:      pipelinev1.ParamTypeString,
			StringVal: "val",
		}},
	}
	originalHeaders := incoming.Header.Clone()

	resPayload, err := i.processEvent(interceptorURL, incoming, payload, params, interceptorTimeout)

	if err != nil {
		t.Errorf("Unexpected error in process event: %q", err)
	}

	if diff := cmp.Diff(payload, resPayload); diff != "" {
		t.Errorf("Did not get expected payload back: %s", diff)
	}

	// Verify that the parameter header was not added to the request header
	if diff := cmp.Diff(incoming.Header, originalHeaders); diff != "" {
		t.Errorf("processEvent() changed request header unexpectedly: %s", diff)
	}
}

func Test_addInterceptorHeaders(t *testing.T) {
	type args struct {
		header       http.Header
		headerParams []pipelinev1.Param
	}
	tests := []struct {
		name string
		args args
		want http.Header
	}{
		{
			name: "Empty params",
			args: args{
				header: map[string][]string{
					"header1": {"val"},
				},
				headerParams: []pipelinev1.Param{},
			},
			want: map[string][]string{
				"header1": {"val"},
			},
		},
		{
			name: "One string param",
			args: args{
				header: map[string][]string{
					"header1": {"val"},
				},
				headerParams: []pipelinev1.Param{
					{
						Name: "header2",
						Value: pipelinev1.ArrayOrString{
							Type:      pipelinev1.ParamTypeString,
							StringVal: "val",
						},
					},
				},
			},
			want: map[string][]string{
				"header1": {"val"},
				"header2": {"val"},
			},
		},
		{
			name: "One array param",
			args: args{
				header: map[string][]string{
					"header1": {"val"},
				},
				headerParams: []pipelinev1.Param{
					{
						Name: "header2",
						Value: pipelinev1.ArrayOrString{
							Type:     pipelinev1.ParamTypeArray,
							ArrayVal: []string{"val1", "val2"},
						},
					},
				},
			},
			want: map[string][]string{
				"header1": {"val"},
				"header2": {"val1", "val2"},
			},
		},
		{
			name: "Clobber param",
			args: args{
				header: map[string][]string{
					"header1": {"val"},
				},
				headerParams: []pipelinev1.Param{
					{
						Name: "header1",
						Value: pipelinev1.ArrayOrString{
							Type:     pipelinev1.ParamTypeArray,
							ArrayVal: []string{"new_val"},
						},
					},
				},
			},
			want: map[string][]string{
				"header1": {"new_val"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addInterceptorHeaders(tt.args.header, tt.args.headerParams)
			if diff := cmp.Diff(tt.want, tt.args.header); diff != "" {
				t.Errorf("addInterceptorHeaders() Diff: -want +got: %s", diff)
			}
		})
	}
}

func TestProcessEvent_TimeOut(t *testing.T) {
	r := Interceptor{HTTPClient: http.DefaultClient}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer ts.Close()

	incoming := httptest.NewRequest("POST", "http://event.listener.url", nil)
	interceptorURL, _ := url.Parse(ts.URL)

	_, err := r.processEvent(interceptorURL, incoming, nil, nil, 10*time.Millisecond)

	if err == nil {
		t.Errorf("Did not expect err to be nil")
	} else if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Unexpected type of error. Expected: deadline exceeded. Got: %q", err)
	}
}
