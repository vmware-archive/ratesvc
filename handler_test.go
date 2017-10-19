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

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/kubeapps/ratesvc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

type body struct {
	Data []item `json:"data"`
}

func TestGetStars(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	currentUser := bson.NewObjectId()
	oldGetCurrentUserID := getCurrentUserID
	getCurrentUserID = func(_ *http.Request) (bson.ObjectId, error) { return currentUser, nil }
	defer func() { getCurrentUserID = oldGetCurrentUserID }()
	tests := []struct {
		name          string
		items         []*item
		starredByUser bool
	}{
		{"no stars", []*item{
			{ID: "stable/wordpress", Type: "chart", StargazersIDs: []bson.ObjectId{}},
		}, false},
		{"stars", []*item{
			{ID: "stable/wordpress", Type: "chart", StargazersIDs: []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}},
			{ID: "stable/drupal", Type: "chart", StargazersIDs: []bson.ObjectId{bson.NewObjectId()}},
		}, false},
		{"starred by user", []*item{
			{ID: "stable/wordpress", Type: "chart", StargazersIDs: []bson.ObjectId{bson.NewObjectId(), currentUser}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.On("All", mock.AnythingOfType("*[]*main.item")).Run(func(args mock.Arguments) {
				*args.Get(0).(*[]*item) = tt.items
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/v1/stars", nil)
			GetStars(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			var b body
			json.NewDecoder(w.Body).Decode(&b)
			require.Len(t, b.Data, len(tt.items))
			for i, it := range tt.items {
				assert.Equal(t, len(it.StargazersIDs), b.Data[i].StargazersCount, "%s stars", it.ID)
				if tt.starredByUser {
					assert.True(t, it.HasStarred)
				}
			}
		})
	}
}

func TestUpdateStar(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	currentUser := bson.NewObjectId()
	oldGetCurrentUserID := getCurrentUserID
	getCurrentUserID = func(_ *http.Request) (bson.ObjectId, error) { return currentUser, nil }
	defer func() { getCurrentUserID = oldGetCurrentUserID }()
	tests := []struct {
		name        string
		requestBody string
		wantCode    int
		unstar      bool
	}{
		{"invalid", `NOTJSON`, http.StatusBadRequest, false},
		{"no id", `{"has_starred": true}`, http.StatusBadRequest, false},
		{"valid", `{"id": "stable/wordpress", "has_starred": true}`, http.StatusCreated, false},
		{"valid unstar", `{"id": "stable/wordpress", "has_starred": false}`, http.StatusCreated, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantCode == http.StatusCreated {
				op := "$push"
				if tt.unstar {
					op = "$pull"
				}
				m.On("UpdateId", "stable/wordpress", bson.M{op: bson.M{"stargazers_ids": currentUser}})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", "/v1/stars", bytes.NewBuffer([]byte(tt.requestBody)))
			UpdateStar(w, req)
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}

func TestUpdateStarUnauthorized(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/v1/stars", nil)
	UpdateStar(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetComments(t *testing.T) {
	type args struct {
		w   http.ResponseWriter
		req *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GetComments(tt.args.w, tt.args.req)
		})
	}
}

func TestCreateComment(t *testing.T) {
	type args struct {
		w   http.ResponseWriter
		req *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CreateComment(tt.args.w, tt.args.req)
		})
	}
}

func Test_getCurrentUserID(t *testing.T) {
	type args struct {
		req *http.Request
	}
	tests := []struct {
		name    string
		args    args
		want    bson.ObjectId
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCurrentUserID(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCurrentUserID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCurrentUserID() = %v, want %v", got, tt.want)
			}
		})
	}
}
