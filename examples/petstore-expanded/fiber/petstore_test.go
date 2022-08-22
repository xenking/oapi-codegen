// Copyright 2019 DeepMap, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/xenking/oapi-codegen/examples/petstore-expanded/fiber/api/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xenking/oapi-codegen/examples/petstore-expanded/fiber/api"
	"github.com/xenking/oapi-codegen/pkg/middleware"
	"github.com/xenking/oapi-codegen/pkg/testutil"
)

func TestPetStore(t *testing.T) {
	var err error
	// Here, we Initialize echo
	app := fiber.New()

	// Now, we create our empty pet store
	store := api.NewPetStore()

	// Get the swagger description of our API
	swagger, err := api.GetSwagger()
	require.NoError(t, err)

	// This disables swagger server name validation. It seems to work poorly,
	// and requires our test server to be in that list.
	swagger.Servers = nil

	// Validate requests against the OpenAPI spec
	app.Use(middleware.OapiRequestValidator(swagger))

	// Log requests
	app.Use(logger.New())

	// We register the autogenerated boilerplate and bind our PetStore to this
	// echo router.
	api.RegisterHandlers(app, store)

	// At this point, we can start sending simulated Http requests, and record
	// the HTTP responses to check for validity. This exercises every part of
	// the stack except the well-tested HTTP system in Go, which there is no
	// point for us to test.
	tag := "TagOfSpot"
	newPet := models.NewPet{
		Name: "Spot",
		Tag:  &tag,
	}
	result := testutil.NewRequest().Post("/pets").WithJsonBody(newPet).GoWithEchoHandler(t, app)
	// We expect 201 code on successful pet insertion
	assert.Equal(t, http.StatusCreated, result.Code())

	// We should have gotten a response from the server with the new pet. Make
	// sure that its fields match.
	var resultPet models.Pet
	err = result.UnmarshalBodyToObject(&resultPet)
	assert.NoError(t, err, "error unmarshaling response")
	assert.Equal(t, newPet.Name, resultPet.Name)
	assert.Equal(t, *newPet.Tag, *resultPet.Tag)

	// This is the Id of the pet we inserted.
	petId := resultPet.ID

	// Test the getter function.
	result = testutil.NewRequest().Get(fmt.Sprintf("/pets/%d", petId)).WithAcceptJson().GoWithEchoHandler(t, app)
	var resultPet2 models.Pet
	err = result.UnmarshalBodyToObject(&resultPet2)
	assert.NoError(t, err, "error getting pet")
	assert.Equal(t, resultPet, resultPet2)

	// We should get a 404 on invalid ID
	result = testutil.NewRequest().Get("/pets/27179095781").WithAcceptJson().GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusNotFound, result.Code())
	var petError models.Error
	err = result.UnmarshalBodyToObject(&petError)
	assert.NoError(t, err, "error getting response", err)
	assert.Equal(t, int32(http.StatusNotFound), petError.Code)

	// Let's insert another pet for subsequent tests.
	tag = "TagOfFido"
	newPet = models.NewPet{
		Name: "Fido",
		Tag:  &tag,
	}
	result = testutil.NewRequest().Post("/pets").WithJsonBody(newPet).GoWithEchoHandler(t, app)
	// We expect 201 code on successful pet insertion
	assert.Equal(t, http.StatusCreated, result.Code())
	// We should have gotten a response from the server with the new pet. Make
	// sure that its fields match.
	err = result.UnmarshalBodyToObject(&resultPet)
	assert.NoError(t, err, "error unmarshaling response")
	petId2 := resultPet.ID

	// Now, list all pets, we should have two
	result = testutil.NewRequest().Get("/pets").WithAcceptJson().GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusOK, result.Code())
	var petList []models.Pet
	err = result.UnmarshalBodyToObject(&petList)
	assert.NoError(t, err, "error getting response", err)
	assert.Equal(t, 2, len(petList))

	// Filter pets by tag, we should have 1
	petList = nil
	result = testutil.NewRequest().Get("/pets?tags=TagOfFido").WithAcceptJson().GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&petList)
	assert.NoError(t, err, "error getting response", err)
	assert.Equal(t, 1, len(petList))

	// Filter pets by non existent tag, we should have 0
	petList = nil
	result = testutil.NewRequest().Get("/pets?tags=NotExists").WithAcceptJson().GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&petList)
	assert.NoError(t, err, "error getting response", err)
	assert.Equal(t, 0, len(petList))

	// Let's delete non-existent pet
	result = testutil.NewRequest().Delete("/pets/7").GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusNotFound, result.Code())
	err = result.UnmarshalBodyToObject(&petError)
	assert.NoError(t, err, "error unmarshaling PetError")
	assert.Equal(t, int32(http.StatusNotFound), petError.Code)

	// Now, delete both real pets
	result = testutil.NewRequest().Delete(fmt.Sprintf("/pets/%d", petId)).GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusNoContent, result.Code())
	result = testutil.NewRequest().Delete(fmt.Sprintf("/pets/%d", petId2)).GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusNoContent, result.Code())

	// Should have no pets left.
	petList = nil
	result = testutil.NewRequest().Get("/pets").WithAcceptJson().GoWithEchoHandler(t, app)
	assert.Equal(t, http.StatusOK, result.Code())
	err = result.UnmarshalBodyToObject(&petList)
	assert.NoError(t, err, "error getting response", err)
	assert.Equal(t, 0, len(petList))
}
