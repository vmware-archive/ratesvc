/*
Copyright (c) 2017 Bitnami

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

package response

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewErrorResponse(t *testing.T) {
	type args struct {
		code    int
		message string
	}
	tests := []struct {
		name string
		args args
		want ErrorResponse
	}{
		{"404 response", args{http.StatusNotFound, "not found"}, ErrorResponse{http.StatusNotFound, "not found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NewErrorResponse(tt.args.code, tt.args.message))
		})
	}
}

func TestErrorResponse_Write(t *testing.T) {
	tests := []struct {
		name string
		e    ErrorResponse
	}{
		{"404 response", ErrorResponse{http.StatusNotFound, "not found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.e.Write(w)
			assert.Equal(t, tt.e.Code, w.Code)
			var body ErrorResponse
			json.NewDecoder(w.Body).Decode(&body)
			assert.Equal(t, tt.e, body)
		})
	}
}

func TestNewDataResponse(t *testing.T) {
	type resource struct {
		ID string
	}
	tests := []struct {
		name string
		data interface{}
	}{
		{"single resource", resource{"test"}},
		{"multiple resources", []resource{{"one"}, {"two"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDataResponse(tt.data)
			assert.Equal(t, tt.data, d.Data)
		})
	}
}

func TestDataResponse_Write(t *testing.T) {
	type resource struct {
		ID string `json:"id"`
	}
	tests := []struct {
		name string
		d    DataResponse
		want string
	}{
		{"single resource", DataResponse{resource{"test"}}, `{"data":{"id":"test"}}`},
		{"multiple resources", DataResponse{[]resource{{"one"}, {"two"}}}, `{"data":[{"id":"one"},{"id":"two"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.d.Write(w)
			assert.Equal(t, http.StatusOK, w.Code)
			bytes, err := ioutil.ReadAll(w.Body)
			assert.NoError(t, err)
			body := string(bytes)
			assert.Equal(t, tt.want, body)
		})
	}
}
