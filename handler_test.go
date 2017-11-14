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
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/kubeapps/ratesvc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

type body struct {
	Data []item `json:"data"`
}

var itemsList []*item

func TestGetStars(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()
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
			{ID: "stable/wordpress", Type: "chart", StargazersIDs: []bson.ObjectId{bson.NewObjectId(), currentUser.ID}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.On("All", &itemsList).Run(func(args mock.Arguments) {
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
	m.On("One", &item{}).Return(nil).Run(func(args mock.Arguments) {
		*args.Get(0).(*item) = item{ID: "stable/wordpress"}
	})
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()
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
				m.On("UpdateId", "stable/wordpress", bson.M{op: bson.M{"stargazers_ids": currentUser.ID}})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", "/v1/stars", bytes.NewBuffer([]byte(tt.requestBody)))
			UpdateStar(w, req)
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}

func TestUpdateStarDoesNotDuplicate(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()

	m.On("One", &item{}).Return(nil).Run(func(args mock.Arguments) {
		*args.Get(0).(*item) = item{ID: "stable/wordpress", StargazersIDs: []bson.ObjectId{currentUser.ID}}
	})

	m.AssertNotCalled(t, "UpdateId")
	w := httptest.NewRecorder()
	requestBody := `{"id": "stable/wordpress", "has_starred": true}`
	req := httptest.NewRequest("PUT", "/v1/stars", bytes.NewBuffer([]byte(requestBody)))
	UpdateStar(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateStarInsertsInexistantItem(t *testing.T) {
	var m mock.Mock
	m.On("One", &item{}).Return(errors.New("not found"))
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()
	tests := []struct {
		name        string
		requestBody string
		unstar      bool
	}{
		{"valid", `{"id": "stable/wordpress", "has_starred": true}`, false},
		{"valid unstar", `{"id": "stable/wordpress", "has_starred": false}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toInsert := item{ID: "stable/wordpress", Type: "chart", HasStarred: !tt.unstar}
			if !tt.unstar {
				toInsert.StargazersIDs = []bson.ObjectId{currentUser.ID}
			}
			m.On("Insert", toInsert)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", "/v1/stars", bytes.NewBuffer([]byte(tt.requestBody)))
			UpdateStar(w, req)
			assert.Equal(t, http.StatusCreated, w.Code)
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
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()

	tests := []struct {
		name     string
		item     item
		cm_count int
	}{
		{"no comments", item{ID: "stable/wordpress", Type: "chart", Comments: []comment{}}, 0},
		{"one comment",
			item{ID: "stable/wordpress", Type: "chart", Comments: []comment{
				{ID: bson.NewObjectId(), Text: "Hello, World!", CreatedAt: time.Now(), Author: currentUser},
			}}, 1},
		{"two comments",
			item{ID: "stable/wordpress", Type: "chart", Comments: []comment{
				{ID: bson.NewObjectId(), Text: "Hello", CreatedAt: time.Now(), Author: currentUser},
				{ID: bson.NewObjectId(), Text: "World!", CreatedAt: time.Now(), Author: currentUser},
			}}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.On("One", &item{}).Return(nil).Run(func(args mock.Arguments) {
				*args.Get(0).(*item) = tt.item
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/v1/comments/stable/wordpress", nil)
			GetComments(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			var b body
			json.NewDecoder(w.Body).Decode(&b)
			require.Len(t, b.Data, tt.cm_count)
		})
	}
}

func TestCreateComment(t *testing.T) {
	var m mock.Mock
	m.On("One", &item{}).Return(nil).Run(func(args mock.Arguments) {
		*args.Get(0).(*item) = item{ID: "stable/wordpress"}
	})
	dbSession = testutil.NewMockSession(&m)
	currentUser := &user{ID: bson.NewObjectId(), Name: "Rick Sanchez", Email: "rick@sanchez.com"}
	oldGetCurrentUser := getCurrentUser
	getCurrentUser = func(_ *http.Request) (*user, error) { return currentUser, nil }
	defer func() { getCurrentUser = oldGetCurrentUser }()

	commentId := getNewObjectID()
	oldGetNewObjectID := getNewObjectID
	getNewObjectID = func() bson.ObjectId { return commentId }
	defer func() { getNewObjectID = oldGetNewObjectID }()

	commentTimestamp := getTimestamp()
	oldGetTimestamp := getTimestamp
	getTimestamp = func() time.Time { return commentTimestamp }
	defer func() { getTimestamp = oldGetTimestamp }()

	tests := []struct {
		name        string
		requestBody string
		wantCode    int
	}{
		{"invalid", `NOTJSON`, http.StatusBadRequest},
		{"no text", `{}`, http.StatusBadRequest},
		{"valid", `{"text": "Hello, World"}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantCode == http.StatusCreated {
				m.On("UpdateId", "stable/wordpress", bson.M{"$push": bson.M{"comments": comment{ID: commentId, Text: "Hello, World", CreatedAt: commentTimestamp, Author: currentUser}}})
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/comments/stable/wordpress", bytes.NewBuffer([]byte(tt.requestBody)))
			CreateComment(w, req)
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}

func TestCreateCommentUnauthorized(t *testing.T) {
	var m mock.Mock
	dbSession = testutil.NewMockSession(&m)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/comments/stable/wordpress", nil)
	CreateComment(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_getCurrentUser(t *testing.T) {
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
			got, err := getCurrentUser(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCurrentUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getCurrentUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
