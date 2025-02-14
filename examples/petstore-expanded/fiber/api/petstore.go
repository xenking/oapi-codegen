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

//go:generate go run github.com/xenking/oapi-codegen/cmd/oapi-codegen --config=types.cfg.yaml ../../petstore-expanded.yaml
//go:generate go run github.com/xenking/oapi-codegen/cmd/oapi-codegen --config=server.cfg.yaml ../../petstore-expanded.yaml
//go:generate go run github.com/xenking/oapi-codegen/cmd/oapi-codegen --config=client.cfg.yaml ../../petstore-expanded.yaml

package api

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/xenking/oapi-codegen/examples/petstore-expanded/fiber/api/models"
	"net/http"
	"sync"
)

type PetStore struct {
	Pets   map[int64]models.Pet
	NextId int64
	Lock   sync.Mutex
}

func NewPetStore() *PetStore {
	return &PetStore{
		Pets:   make(map[int64]models.Pet),
		NextId: 1000,
	}
}

// This function wraps sending of an error in the Error format, and
// handling the failure to marshal that.
func sendPetstoreError(ctx *fiber.Ctx, code int, message string) error {
	petErr := models.Error{
		Code:    int32(code),
		Message: message,
	}
	err := ctx.Status(code).JSON(petErr)
	return err
}

// Here, we implement all of the handlers in the ServerInterface
func (p *PetStore) FindPets(ctx *fiber.Ctx, params models.FindPetsParams) error {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	var result []models.Pet

	for _, pet := range p.Pets {
		if params.Tags != nil {
			// If we have tags,  filter pets by tag
			for _, t := range *params.Tags {
				if pet.Tag != nil && (*pet.Tag == t) {
					result = append(result, pet)
				}
			}
		} else {
			// Add all pets if we're not filtering
			result = append(result, pet)
		}

		if params.Limit != nil {
			l := int(*params.Limit)
			if len(result) >= l {
				// We're at the limit
				break
			}
		}
	}
	return ctx.Status(http.StatusOK).JSON(result)
}

func (p *PetStore) AddPet(ctx *fiber.Ctx) error {
	// We expect a NewPet object in the request body.
	var newPet models.NewPet
	err := ctx.BodyParser(&newPet)
	if err != nil {
		return sendPetstoreError(ctx, http.StatusBadRequest, "Invalid format for NewPet")
	}
	// We now have a pet, let's add it to our "database".

	// We're always asynchronous, so lock unsafe operations below
	p.Lock.Lock()
	defer p.Lock.Unlock()

	// We handle pets, not NewPets, which have an additional ID field
	var pet models.Pet
	pet.Name = newPet.Name
	pet.Tag = newPet.Tag
	pet.ID = p.NextId
	p.NextId = p.NextId + 1

	// Insert into map
	p.Pets[pet.ID] = pet

	// Now, we have to return the NewPet
	err = ctx.Status(http.StatusCreated).JSON(pet)
	if err != nil {
		// Something really bad happened, tell Echo that our handler failed
		return err
	}

	// Return no error. This refers to the handler. Even if we return an HTTP
	// error, but everything else is working properly, tell Echo that we serviced
	// the error. We should only return errors from Echo handlers if the actual
	// servicing of the error on the infrastructure level failed. Returning an
	// HTTP/400 or HTTP/500 from here means Echo/HTTP are still working, so
	// return nil.
	return nil
}

func (p *PetStore) FindPetByID(ctx *fiber.Ctx, petId int64) error {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	pet, found := p.Pets[petId]
	if !found {
		return sendPetstoreError(ctx, http.StatusNotFound,
			fmt.Sprintf("Could not find pet with ID %d", petId))
	}
	return ctx.Status(http.StatusOK).JSON(pet)
}

func (p *PetStore) DeletePet(ctx *fiber.Ctx, id int64) error {
	p.Lock.Lock()
	defer p.Lock.Unlock()

	_, found := p.Pets[id]
	if !found {
		return sendPetstoreError(ctx, http.StatusNotFound,
			fmt.Sprintf("Could not find pet with ID %d", id))
	}
	delete(p.Pets, id)
	return ctx.SendStatus(http.StatusNoContent)
}
