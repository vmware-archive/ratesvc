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
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/kubeapps/ratesvc/response"
	log "github.com/sirupsen/logrus"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/globalsign/mgo/bson"
)

// Params a key-value map of path params
type Params map[string]string

// WithParams can be used to wrap handlers to take an extra arg for path params
type WithParams func(http.ResponseWriter, *http.Request, Params)

func (h WithParams) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	h(w, req, vars)
}

const itemCollection = "items"

type item struct {
	// Instead of bson.ObjectID, we use a human-friendly identifier (e.g. "stable/wordpress")
	ID string `json:"id" bson:"_id,omitempty"`
	// Type could be "chart", "function", etc.
	Type string `json:"type"`
	// List of IDs of Stargazers that will be stored in the database
	StargazersIDs []bson.ObjectId `json:"-" bson:"stargazers_ids"`
	// Count of the Stargazers which is only exposed in the JSON response
	StargazersCount int `json:"stargazers_count" bson:"-"`
	// Whether the current user has starred the item, only exposed in the JSON response
	HasStarred bool `json:"has_starred" bson:"-"`
	// Comments collection
	Comments []comment `json:"-"`
}

// User represents user info
type User struct {
	ID        bson.ObjectId `json:"id" bson:"_id"`
	Name      string        `json:"name"`
	Email     string        `json:"-"`
	AvatarURL string        `json:"avatar_url" bson:"-"`
}

// Defines a comment object
type comment struct {
	ID        bson.ObjectId `json:"id" bson:"_id,omitempty"`
	Text      string        `json:"text"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at"`
	Author    *User         `json:"author"`
}

// GetStars returns a list of starred items
func GetStars(w http.ResponseWriter, req *http.Request) {
	db, closer := dbSession.DB()
	defer closer()
	var items []*item
	if err := db.C(itemCollection).Find(nil).All(&items); err != nil {
		log.WithError(err).Error("could not fetch all items")
		response.NewErrorResponse(http.StatusInternalServerError, "could not fetch all items").Write(w)
		return
	}

	currentUser, _ := getCurrentUser(req)
	for _, it := range items {
		it.StargazersCount = len(it.StargazersIDs)
		if currentUser != nil {
			it.HasStarred = hasStarred(it, currentUser)
		}
	}
	response.NewDataResponse(items).Write(w)
}

// UpdateStar updates the HasStarred attribute on an item
func UpdateStar(w http.ResponseWriter, req *http.Request) {
	db, closer := dbSession.DB()
	defer closer()

	currentUser, err := getCurrentUser(req)
	if err != nil {
		response.NewErrorResponse(http.StatusUnauthorized, "unauthorized").Write(w)
		return
	}

	// Params validation
	var params *item
	if err := json.NewDecoder(req.Body).Decode(&params); err != nil {
		log.WithError(err).Error("could not parse request body")
		response.NewErrorResponse(http.StatusBadRequest, "could not parse request body").Write(w)
		return
	}

	if params.ID == "" {
		response.NewErrorResponse(http.StatusBadRequest, "id missing in request body").Write(w)
		return
	}

	if params.Type == "" {
		params.Type = "chart"
	}

	var it item
	err = db.C(itemCollection).FindId(params.ID).One(&it)

	if err != nil {
		// Create the item if inexistant
		it = *params
		if params.HasStarred {
			it.StargazersIDs = []bson.ObjectId{currentUser.ID}
		}
		if err := db.C(itemCollection).Insert(it); err != nil {
			log.WithError(err).Error("could not insert item")
			response.NewErrorResponse(http.StatusInternalServerError, "internal server error").Write(w)
			return
		}
	} else {
		// Otherwise we just need to update the database
		op := "$pull"
		if params.HasStarred {
			// no-op if item is already starred by user
			if hasStarred(&it, currentUser) {
				response.NewDataResponse(it).WithCode(http.StatusOK).Write(w)
				return
			}
			op = "$push"
		}

		if err := db.C(itemCollection).UpdateId(it.ID, bson.M{op: bson.M{"stargazers_ids": currentUser.ID}}); err != nil {
			log.WithError(err).Error("could not update item")
			response.NewErrorResponse(http.StatusInternalServerError, "internal server error").Write(w)
			return
		}
	}

	response.NewDataResponse(it).WithCode(http.StatusCreated).Write(w)
}

// GetComments returns a list of comments
func GetComments(w http.ResponseWriter, req *http.Request, params Params) {
	db, closer := dbSession.DB()
	defer closer()

	var it item
	itemID := params["repo"] + "/" + params["chartName"]
	if err := db.C(itemCollection).FindId(itemID).One(&it); err != nil {
		response.NewDataResponse([]int64{}).Write(w)
		return
	}

	for _, cm := range it.Comments {
		h := md5.New()
		io.WriteString(h, cm.Author.Email)
		cm.Author.AvatarURL = fmt.Sprintf("https://s.gravatar.com/avatar/%x", h.Sum(nil))
	}
	response.NewDataResponse(it.Comments).Write(w)
}

// CreateComment creates a comment and appends the comment to the item.Comments array
func CreateComment(w http.ResponseWriter, req *http.Request, params Params) {
	db, closer := dbSession.DB()
	defer closer()

	currentUser, err := getCurrentUser(req)
	if err != nil {
		response.NewErrorResponse(http.StatusUnauthorized, "unauthorized").Write(w)
		return
	}

	// Params validation
	var cm comment
	if err := json.NewDecoder(req.Body).Decode(&cm); err != nil {
		log.WithError(err).Error("could not parse request body")
		response.NewErrorResponse(http.StatusBadRequest, "could not parse request body").Write(w)
		return
	}

	if cm.Text == "" {
		response.NewErrorResponse(http.StatusBadRequest, "text missing in request body").Write(w)
		return
	}

	cm.ID = getNewObjectID()
	cm.CreatedAt = getTimestamp()
	cm.Author = currentUser

	var it item
	itemID := params["repo"] + "/" + params["chartName"]
	if err = db.C(itemCollection).FindId(itemID).One(&it); err != nil {
		// Create the item if inexistant
		it.Type = "chart"
		it.ID = itemID
		it.Comments = []comment{cm}
		if err := db.C(itemCollection).Insert(it); err != nil {
			log.WithError(err).Error("could not insert item")
			response.NewErrorResponse(http.StatusInternalServerError, "internal server error").Write(w)
			return
		}
	} else {
		// Append comment to collection
		if err = db.C(itemCollection).UpdateId(it.ID, bson.M{"$push": bson.M{"comments": cm}}); err != nil {
			log.WithError(err).Error("could not update item")
			response.NewErrorResponse(http.StatusInternalServerError, "internal server error").Write(w)
			return
		}
	}

	// update avatar_url in response object
	h := md5.New()
	io.WriteString(h, cm.Author.Email)
	cm.Author.AvatarURL = fmt.Sprintf("https://s.gravatar.com/avatar/%x", h.Sum(nil))

	response.NewDataResponse(cm).WithCode(http.StatusCreated).Write(w)
}

// DeleteComment delete's an existing comment
func DeleteComment(w http.ResponseWriter, req *http.Request, params Params) {
	db, closer := dbSession.DB()
	defer closer()

	itemID := params["repo"] + "/" + params["chartName"]
	commentID := bson.ObjectIdHex(params["commentId"])

	currentUser, err := getCurrentUser(req)
	if err != nil {
		response.NewErrorResponse(http.StatusUnauthorized, "unauthorized").Write(w)
		return
	}

	var it item
	if err := db.C(itemCollection).FindId(itemID).One(&it); err != nil {
		response.NewErrorResponse(http.StatusNotFound, "comment not found").Write(w)
		return
	}

	var cm comment
	for _, c := range it.Comments {
		if commentID == c.ID {
			cm = c
			break
		}
	}

	if cm == (comment{}) {
		response.NewErrorResponse(http.StatusNotFound, "comment not found").Write(w)
		return
	}

	// Users can only delete their own comments
	if cm.Author.ID != currentUser.ID {
		response.NewErrorResponse(http.StatusUnauthorized, "not authorized to delete this comment").Write(w)
		return
	}

	if err = db.C(itemCollection).UpdateId(it.ID, bson.M{"$pull": bson.M{"comments": cm}}); err != nil {
		response.NewErrorResponse(http.StatusInternalServerError, "internal server error").Write(w)
		return
	}
	response.NewDataResponse(cm).WithCode(http.StatusAccepted).Write(w)
}

type userClaims struct {
	*User
	Email string
	jwt.StandardClaims
}

var getCurrentUser = func(req *http.Request) (*User, error) {
	jwtKey, ok := os.LookupEnv("JWT_KEY")
	if !ok {
		return nil, errors.New("JWT_KEY not set")
	}

	cookie, err := req.Cookie("ka_auth")
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &userClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtKey), nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*userClaims); ok && token.Valid {
		claims.User.Email = claims.Email
		return claims.User, nil
	}
	return nil, errors.New("invalid token")
}

var getNewObjectID = func() bson.ObjectId {
	return bson.NewObjectId()
}

var getTimestamp = func() time.Time {
	return time.Now()
}

// hasStarred returns true if item is starred by the user
func hasStarred(it *item, currentUser *User) bool {
	for _, id := range it.StargazersIDs {
		if id == currentUser.ID {
			return true
		}
	}
	return false
}
