package handlers

import (
	"context"
	"errors"
	//"log"
	"net/http"
	"strings"
	"time"
	//"regexp"
	"net/mail"

	"example.com/my/module/database"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        string    `json:"id" bson:"id"`
	Firstname string    `json:"firstname" bson:"firstname"`
	Lastname  string    `json:"lastname" bson:"lastname"`
	Email     string    `json:"email" bson:"email"`
	Password  string    `json:"password" bson:"password"`
	Confirmpassword string `json:"confirmpassword" bson:"confirmpassword"`
	CreatedAt time.Time `json:"createdat" bson:"createdat"`
}

func validMailAddress(address string) bool {
	_, err := mail.ParseAddress(address)
	return err == nil
}

func RegisterHandler(c *gin.Context) {
	var user User
	user.CreatedAt = time.Now()

	// Bind JSON data to the user struct
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// Trim whitespace from user input
	user.Firstname = strings.TrimSpace(user.Firstname)
	user.Lastname = strings.TrimSpace(user.Lastname)
	user.Email = strings.TrimSpace(user.Email)
	user.Password = strings.TrimSpace(user.Password)
	user.Confirmpassword=strings.TrimSpace(user.Confirmpassword)


	// Check if any of the required fields are empty
	if user.Firstname == "" || user.Lastname == "" || user.Email == "" || user.Password == "" || user.Confirmpassword==""{
		c.JSON(http.StatusBadRequest, gin.H{"error": "All fields are mandatory"})
		return
	}

	if !validMailAddress(user.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
		return
	}

	var existingUser User
	// Check if the user already exists based on firstname, lastname, or email
	if err := database.UserCollection.FindOne(context.TODO(), bson.M{
		"$and": []bson.M{
			{"firstname": user.Firstname},
			{"lastname": user.Lastname},
			{"email": user.Email},
		},
	}).Decode(&existingUser); err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User already exists"})
		return
	}

	// Generate a new UUID for the user ID
	user.ID = uuid.New().String()

	if user.Password!=user.Confirmpassword{
		c.JSON(http.StatusBadRequest, gin.H{"error":"Password and confirm password do not match"})
		//log.Fatal(user.Password);
		
		return
	}
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash the password"})
		return
	}
	user.Password = string(hashedPassword)

	// Insert the user into the database

	
	_, err = database.UserCollection.InsertOne(context.Background(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
	
}

func LoginHandler(c *gin.Context) {
	var user User
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	filter := bson.M{"email": user.Email}

	var result User
	if err := database.UserCollection.FindOne(context.TODO(), filter).Decode(&result); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not registered"})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		}
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(result.Password), []byte(user.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	session := sessions.Default(c)
	session.Set("userID", result.ID)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
		return
	}
	session.Save()
	sessionID := session.ID()

	// Set the session cookie in the response headers
	c.SetCookie("mysession", sessionID, 3600, "/", "localhost", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

func ProfileHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("userID")
	if userID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not logged in"})
		return
	}
	var user User
	filter := bson.M{"id": userID.(string)} // Convert userID to string if it's not already
	if err := database.UserCollection.FindOne(context.TODO(), filter).Decode(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Profile Accessed", "user": user})
}

func LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear session"})
		return
	}

	// Remove the session cookie from the response headers
	c.SetCookie("mysession", "", -1, "/", "localhost", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}




func GetAllUsers(c *gin.Context) {
    filter := bson.M{}
    var users []User
    cursor, err := database.UserCollection.Find(context.Background(), filter)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer cursor.Close(context.Background())

    for cursor.Next(context.Background()) {
        var user User
        if err := cursor.Decode(&user); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        users = append(users, user)
    }

    if err := cursor.Err(); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, users)
}


func GetSpecificUser(c *gin.Context) {
    // Retrieve the email query parameter from the request
    email := c.Query("email")

    // Define a filter to retrieve documents with the specified email
    filter := bson.M{}
    if email != "" {
        filter = bson.M{"email": email}
    }

    // Create a slice to store the results
    var users []User

    // Perform the find operation with the filter
    cursor, err := database.UserCollection.Find(context.Background(), filter)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer cursor.Close(context.Background())

    // Iterate over the cursor and decode each document into a User struct
    for cursor.Next(context.Background()) {
        var user User
        if err := cursor.Decode(&user); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        users = append(users, user)
    }

    // Check if any error occurred during cursor iteration
    if err := cursor.Err(); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Return the list of users as JSON response
    c.JSON(http.StatusOK, users)
}