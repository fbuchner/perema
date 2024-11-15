package controllers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"perema/models"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func CreateContact(c *gin.Context) {
	var contact models.Contact
	if err := c.ShouldBindJSON(&contact); err != nil {
		log.Println("Error binding JSON for create contact:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Save to the database
	db := c.MustGet("db").(*gorm.DB)

	// Save the new contact to the database
	if err := db.Create(&contact).Error; err != nil {
		log.Println("Error saving to database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save contact"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact created successfully", "contact": contact})
}

// GetAllContacts handles GET requests to fetch contact records with optional field selection, relationships, and pagination.
//
// Query Parameters:
// - `page` (int, optional): The page number for pagination (default: 1).
// - `limit` (int, optional): The number of records per page (default: 25).
// - `fields` (string, optional): A comma-separated list of fields to include in the response.
//   - Example: "firstname,lastname,email"
//   - If omitted, all fields are included.
//
// - `includes` (string, optional): A comma-separated list of related data to preload.
//   - Example: "notes,activities"
//   - If omitted, no relationships are preloaded.
//
// Response:
//   - JSON object with the following structure:
//     {
//     "contacts": [ /* Array of contact records */ ],
//     "total": <total number of contacts>,
//     "page": <current page number>,
//     "limit": <number of records per page>
//     }
//
// Error Handling:
// - Returns HTTP 500 with an error message if the database query fails.
//
// Example Requests:
// - Fetch all fields: GET /contacts?page=1&limit=10
// - Fetch specific fields: GET /contacts?fields=firstname,lastname,email&page=1&limit=5
// - Fetch relationships: GET /contacts?include=notes,activities
func GetAllContacts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 25
	}
	offset := (page - 1) * limit

	// Parse requested fields
	fields := c.Query("fields") // Example: "firstname,lastname,email"
	var selectedFields []string
	includeAllFields := fields == ""

	if !includeAllFields {
		selectedFields = strings.Split(fields, ",")
	}

	// Parse relationships to include
	includes := c.Query("includes") // Example: "notes,activities"
	includedRelationships := strings.Split(includes, ",")
	relationshipMap := map[string]bool{
		"notes":      false,
		"activities": false,
	}

	for _, rel := range includedRelationships {
		if _, exists := relationshipMap[rel]; exists {
			relationshipMap[rel] = true
		}
	}

	// Base query
	var contacts []models.Contact

	query := db.Model(&models.Contact{}).Limit(limit).Offset(offset)

	// Include all fields if none are specified
	if !includeAllFields {
		query = query.Select(strings.Join(selectedFields, ", "))
	}

	// Preload requested relationships
	if relationshipMap["notes"] {
		query = query.Preload("Notes")
	}
	if relationshipMap["activities"] {
		query = query.Preload("Activities")
	}

	// Execute query
	if err := query.Find(&contacts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve contacts"})
		return
	}

	// Get total count of contacts
	var total int64
	db.Model(&models.Contact{}).Count(&total)

	// Respond with contacts and pagination metadata
	c.JSON(http.StatusOK, gin.H{
		"contacts": contacts,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

func GetContact(c *gin.Context) {
	id := c.Param("id")
	var contact models.Contact
	db := c.MustGet("db").(*gorm.DB)
	if err := db.Preload("Notes").Preload("Activities").First(&contact, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contact not found"})
		return
	}

	c.JSON(http.StatusOK, contact)
}

func UpdateContact(c *gin.Context) {
	id := c.Param("id")
	var contact models.Contact
	db := c.MustGet("db").(*gorm.DB)
	if err := db.First(&contact, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contact not found"})
		return
	}

	if err := c.ShouldBindJSON(&contact); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db.Save(&contact)
	c.JSON(http.StatusOK, contact)
}

func DeleteContact(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	if err := db.Delete(&models.Contact{}, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contact not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact deleted"})
}

func AddProfilePictureToContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Parse contact ID from the request parameters
	contactIDParam := c.Param("id")
	contactID, err := strconv.Atoi(contactIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid contact ID"})
		return
	}

	// Find the contact by ID
	var contact models.Contact
	if err := db.First(&contact, contactID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Contact not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find contact"})
		return
	}

	// Get the uploaded file
	file, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to upload file"})
		return
	}

	// Define the file path to save the uploaded file
	uploadDir := os.Getenv("PROFILE_PHOTO_DIR")
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Sanitize the filename to prevent directory traversal
	filename := filepath.Base(file.Filename)
	if strings.Contains(filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file name"})
		return
	}

	filePath := filepath.Join(uploadDir, file.Filename)

	// Save the uploaded file to the server
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Update the contact's photo field
	contact.Photo = filePath
	if err := db.Save(&contact).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update contact photo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile picture added successfully"})
}

func AddRelationshipToContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Parse contact ID from the request parameters
	contactIDParam := c.Param("id")
	contactID, err := strconv.Atoi(contactIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid contact ID"})
		return
	}

	// Find the contact by ID
	var contact models.Contact
	if err := db.First(&contact, contactID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Contact not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find contact"})
		return
	}

	// Bind the request body to the Relationship struct
	var relationship models.Relationship
	if err := c.ShouldBindJSON(&relationship); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// If the relationship is linked to an existing contact, validate the contact
	if relationship.ContactID != nil {
		var relatedContact models.Contact
		if err := db.First(&relatedContact, *relationship.ContactID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "Related contact not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find related contact"})
			return
		}
		relationship.RelatedContact = &relatedContact
	}

	// Append the relationship to the contact
	contact.Relationships = append(contact.Relationships, relationship)

	// Save the contact with the new relationship
	if err := db.Save(&contact).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add relationship to contact"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Relationship added to contact successfully"})
}

// GetCircles returns all unique circles associated with contacts.
func GetCircles(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var circleNames []string

	// Raw SQL query to extract unique circle names
	err := db.Raw(`SELECT DISTINCT json_each.value AS circle 
	               FROM contacts, json_each(contacts.circles)`).Scan(&circleNames).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve circles"})
		return
	}

	// Return the list of unique circle names
	c.JSON(http.StatusOK, circleNames)
}
