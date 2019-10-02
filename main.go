package main

import (
    "os"
    "fmt"
	"log"
	"errors"
	"strings"
	"context"
	"encoding/json"
    "database/sql"
    _ "github.com/lib/pq"
	"github.com/google/uuid"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/events"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func makeErrorResponse(status int, code string, message string) (events.APIGatewayProxyResponse, error) {
	payload, _ := json.Marshal(errorResponse{code, message})
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body: string(payload),
		IsBase64Encoded: false,
	}, nil
}

type contactMutation struct {
    *sql.Tx
}

func contactExists(contactID *uuid.UUID) (bool) {
    cursor, err := conn.Query("SELECT contact_id FROM contact WHERE contact_id = $1", contactID)
    return err == nil && cursor.Next() && cursor.Close() == nil
}

func (db contactMutation) upsertContactPhoneNumbers(contact *Contact) (int64, error) {
    var total int64 = 0
    for phoneType, number := range contact.Numbers {
        //TODO: refactor contact_phone exists check logic into a separate function
        cursor, err := db.Query("SELECT count(*) FROM contact_phone WHERE contact_id = $1 AND contact_phone_type = $2", contact.ID, phoneType)
        if (err == nil && !cursor.Next()) {
            err = errors.New("No result count delivered from database!")
        }
        if (err != nil) {
            return 0, err
        }
        var upsertStatement string
        var exists int
        cursor.Scan(&exists)
        cursor.Close()
        if (exists == 0) {
            upsertStatement = "INSERT INTO contact_phone (number, contact_id, contact_phone_type) VALUES ($1, $2, $3)"
        } else {
            upsertStatement = "UPDATE contact_phone SET number = $1 WHERE contact_id = $2 AND contact_phone_type = $3"
        }
        stmt, err := db.Prepare(upsertStatement)
        if (err != nil) {
            return 0, err
        }
        insert, err := stmt.Exec(number, contact.ID, phoneType)
        if (err != nil) {
            return 0, err
        }
        result, err := insert.RowsAffected()
        if (err != nil) {
            return 0, err
        }
        total += result
    }
    return total, nil
}

func (db contactMutation) mutateContact(template string, contact *Contact) (error) {
    stmt, err := db.Prepare(template)
    if (err != nil) {
        return err
    }
    result, err := stmt.Exec(contact.Name, contact.Email, contact.ID)
    if (err != nil) {
        return err
    }
    success, err := result.RowsAffected()
    if (err != nil) {
        return err
    }
    err = stmt.Close()
    if (err != nil) {
        return err
    }
    if (success == 1) {
        return nil
    }
    return errors.New("Mismatch record count returned from database!")
}

func saveContact(contact *Contact) (error) {
    db, err := conn.Begin()
    if (err != nil) {
        log.Println("Error starting transaction!")
        log.Println(err);
        return err
    }
    tx := contactMutation{db}
    ready := false
    for {  //GOTO
        if (contactExists(&contact.ID)) {
            err = tx.mutateContact("UPDATE contact SET name = $1, email = $2 WHERE contact_id = $3",contact)
        } else {
            err = tx.mutateContact("INSERT INTO contact (name, email, contact_id) VALUES ($1, $2, $3)",contact)
        }
        if (err != nil) {
            log.Println("Error saving contact record")
            log.Println(err);
            break
        }
        log.Println("Contact record saved successfully")
        _, err := tx.upsertContactPhoneNumbers(contact)
        if (err != nil) {
            log.Println(err)
            log.Println("Error in phone number upsert!")
            break
        }
        log.Println("Updated contact phone numbers successfully")
        ready = true
        break
    }
    if (ready) {
        log.Println("Committing changes")
        err = tx.Commit()
    } else {
        log.Println("Rolling back contact changes")
        e := tx.Rollback()
        if (e != nil) {
            return e
        }
    }
    if (err != nil) {
        log.Println("Error trying to commit changes!")
        return err
    }
    return nil
}

type contactPayload string
type PhoneMap map[string]string;
var PhoneType = [...]string {"home", "work", "mobile", "fax"}  //Yeah... There's probably a better way to do an enum in Go?
type Contact struct {
  ID      uuid.UUID `json:"contact_id"`
  Name    string
  Email   string
  Numbers PhoneMap
}

func IsValidPhoneNumber(number string) (bool) {
    if (len(number) > 15 || len(number) < 9) {
        return false
    }
    for _, c := range number {
        if (c < '0' || c > '9') {
            return false
        }
    }
    return true
}

func (phones PhoneMap) ValidatePhoneMap() (error) {
    if (len(phones) == 0) {
        return errors.New("Missing Contact Phone Number(s)")
    }
    var validKey bool
    for key, num := range phones {
        validKey = false
        for _, typ := range PhoneType {  //Same thing, this feels like an unneccessary iteration right here
            if (key == typ) {
                validKey = true
                if (!IsValidPhoneNumber(num)) {
                    return errors.New(fmt.Sprintf("Invalid Phone Number specified for \"%s\": %s", typ, num))
                }
            }
        }
        if (!validKey) {
            return errors.New(fmt.Sprintf("Contact Phone Number Type invalid: %s", key))
        }
    }
	return nil
}

func (body contactPayload) UnmarshalValidContact() (*Contact, error) {
	contact := Contact{}
	invalidJSON := json.Unmarshal([]byte(string(body)), &contact)
	if (invalidJSON != nil) {
		return nil, invalidJSON
	}
	if (contact.Name == "") {
		return nil, errors.New("Missing Contact Name")
	}
	if (contact.Email == "" || len(contact.Email) < 3) {
		return nil, errors.New("Missing Contact Email")
	}
	if (!strings.ContainsRune(contact.Email, '@')) {
		return nil, errors.New("Invalid Contact Email")
	}
	return &contact, contact.Numbers.ValidatePhoneMap()
}

func Handler(context context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if (request.HTTPMethod != "POST" && request.HTTPMethod != "PUT") {
		log.Printf("Request failing due to incorrect HTTP verb: %s", request.HTTPMethod)
		return makeErrorResponse(405, "Method Not Allowed", "Only POST and PUT methods are accepted.")
	}
	if (request.Headers["Content-type"] != "application/json") {
		log.Printf("Request failing due to incorrect Content-type: %s", request.Headers["Content-type"])
		return makeErrorResponse(415,"Unsupported Media Type","Content-type must be application/json!")
	}
	contact, badData := contactPayload(request.Body).UnmarshalValidContact()
	if (badData != nil) {
		log.Printf("Request failing due to invalid data payload. %s", badData.Error())
		return makeErrorResponse(400,"Bad Request",badData.Error());
	}
	log.Printf("Successfully unmarshalled and valiated contact payload: %+v", contact)
    exists := contactExists(&contact.ID)
	if (exists && request.HTTPMethod == "POST") {
		return makeErrorResponse(403,"Forbidden","Cannot create a contact with identical id to existing contact!")
	}
    notSaved := saveContact(contact)
    if (notSaved != nil) {
        log.Printf("Request failing due to persistence failure. %s", notSaved.Error())
		return makeErrorResponse(501,"Service Unavailable",notSaved.Error());
    }
	if (exists) {
		return events.APIGatewayProxyResponse{StatusCode: 204}, nil
	} else {
		return events.APIGatewayProxyResponse{StatusCode: 201}, nil
	}
}

var conn *sql.DB

func main() {
    os.Setenv("PG_CONNECTION_STRING","user=postgres password=postgres dbname=contact sslmode=disable")
    var err error
	//get DB connection
    conn, err = sql.Open("postgres", os.Getenv("PG_CONNECTION_STRING"))
    if (err != nil) {
        panic(err)
    }
    //Begin accepting invoke
	lambda.Start(Handler)
    conn.Close()
    //I wasn't 100% sure of how panic/defer interacted with the Lambda model in terms of process lifecycle, so I just did this hack to make sure the main thread closes the database connection after the lambda runtime returns on the main thread.
}