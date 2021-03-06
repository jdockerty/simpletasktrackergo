package main

import (
	"encoding/json"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
)

const (
	currentRuntime = runtime.GOOS
)

// Task struct for use in templates.
type Task struct {
	TaskID         string
	TaskName       string
	TaskDetails    string
	CompletionDate string
}

// Tasks are a slice of Task, used for populating data to place into templates.
type Tasks []Task

// Function for adding a task to the task database, the user does not need to generate a taskID as this is done using
// a new UUID so that it is compliant and saves the user time. An example is shown below.
// {
//	"TaskName": "My New Task",
//	"TaskDetails": "New Task Details",
//	"CompletionDate": "31/12/2020"
//	}
func addTaskAPI(w http.ResponseWriter, r *http.Request) {
	var newTask Task
	requestBody, _ := ioutil.ReadAll(r.Body)
	json.Unmarshal(requestBody, &newTask)
	newTask.TaskID = uuid.New().String()
	addTaskAPIHelper(newTask)
	json.NewEncoder(w).Encode("Task added, ID is " + newTask.TaskID)
}

// Helper function for adding a new task from the add task API portion.
func addTaskAPIHelper(newTask Task) {
	dbSession := awsConnection()

	itemInput := &dynamodb.PutItemInput{
		TableName: aws.String("Task-Tracker"),
		Item: map[string]*dynamodb.AttributeValue{
			"TaskID": {
				S: aws.String(newTask.TaskID),
			},
			"Task Name": {
				S: aws.String(newTask.TaskName),
			},
			"Task Details": {
				S: aws.String(newTask.TaskDetails),
			},
			"Completion Date": {
				S: aws.String(newTask.CompletionDate),
			},
		},
	}

	_, err := dbSession.PutItem(itemInput)
	if err != nil {
		panic(err)
	}

}

// Function for serving the add task page. Data is sent from the form to AWS through a POST request.
func addTasks(w http.ResponseWriter, r *http.Request) {

	if currentRuntime == "windows" {
		template := template.Must(template.ParseFiles(`webpage\addtasks.html`))
		template.Execute(w, nil)
	} else {
		template := template.Must(template.ParseFiles(`webpage/addtasks.html`))
		template.Execute(w, nil)
	}

	r.ParseForm()
	if r.Method == "POST" {
		newTaskID := uuid.New()
		newTask := Task{TaskID: newTaskID.String(),
			TaskDetails:    r.FormValue("TaskDetails"),
			TaskName:       r.FormValue("TaskName"),
			CompletionDate: r.FormValue("CompleteBy")}
		dbSession := awsConnection()

		itemInput := &dynamodb.PutItemInput{
			TableName: aws.String("Task-Tracker"),
			Item: map[string]*dynamodb.AttributeValue{
				"TaskID": {
					S: aws.String(newTask.TaskID),
				},
				"Task Name": {
					S: aws.String(newTask.TaskName),
				},
				"Task Details": {
					S: aws.String(newTask.TaskDetails),
				},
				"Completion Date": {
					S: aws.String(newTask.CompletionDate),
				},
			},
		}

		_, err := dbSession.PutItem(itemInput)
		if err != nil {
			panic(err)
		}

		log.Printf("\nTask sent: \n\tTaskID = %s\n\tTask Name = %s\n\tTask Details = %s\n\tCompletion Date = %s\n\n",
			newTask.TaskID, newTask.TaskName, newTask.TaskDetails, newTask.CompletionDate)
	}

}

// getParams will pull the AWS credentials from SSM Parameter store, these can then be passed to
// read any data from the DynamoDB table.
func getParams(sess *session.Session) *credentials.Credentials {
	var creds []string
	ssmsvc := ssm.New(sess, aws.NewConfig().WithRegion("eu-west-2"))
	params, err := ssmsvc.GetParameters(&ssm.GetParametersInput{Names: []*string{aws.String("access_key"), aws.String("s_access")}})
	if err != nil {
		panic(err)
	}

	creds = append(creds, *params.Parameters[0].Value, *params.Parameters[1].Value)
	c := credentials.NewStaticCredentials(creds[0], creds[1], "")
	if err != nil {
		panic(err)

	}

	return c
}

// Helper function for setting up the AWS connection.
func awsConnection() *dynamodb.DynamoDB {
	session, err := session.NewSession(&aws.Config{Region: aws.String("eu-west-2")})
	credentials := getParams(session)
	if err != nil {
		panic(err)
	}

	dbInstance := dynamodb.New(session, &aws.Config{Credentials: credentials})
	return dbInstance
}

// Function for serving the JSON response to a HTTP GET request with a task information, this is serves an array of JSON which can
// be unmarshalled back into the Tasks slice shown here.
func viewAllTasksAPI(w http.ResponseWriter, r *http.Request) {
	allTasks := populateTasks()

	if len(allTasks) == 0 {
		json.NewEncoder(w).Encode("No tasks available.")
	} else {
		json.NewEncoder(w).Encode(allTasks)
	}
}

// Helper function for populating and returning a list of task structs.
func populateTasks() Tasks {
	input := &dynamodb.ScanInput{
		TableName: aws.String("Task-Tracker"),
	}
	dbSession := awsConnection()

	allData, err := dbSession.Scan(input)
	if err != nil {
		panic(err)
	}

	if len(allData.Items) == 0 {
		log.Println("Table is empty.")
	} else {
		var myTasks Tasks
		for _, value := range allData.Items {
			task := Task{*value["TaskID"].S, *value["Task Name"].S, *value["Task Details"].S, *value["Completion Date"].S}
			myTasks = append(myTasks, task)
		}

		return myTasks

	}
	return nil
}

// Function for serving the view tasks page. All of the data is scanned from DynamoDB and a table is dynamically generated through a template.
func viewTasks(w http.ResponseWriter, r *http.Request) {
	allTasks := populateTasks()

	if currentRuntime == "windows" {
		template := template.Must(template.ParseFiles(`webpage\viewtasks.html`))
		template.Execute(w, allTasks)
	} else {
		template := template.Must(template.ParseFiles(`webpage/viewtasks.html`))
		template.Execute(w, allTasks)
	}
}

// Function for serving the modify task page. All data is scanned and taskIDs populate the selection box for the user to select the task they wish to modify.
func modifyTask(w http.ResponseWriter, r *http.Request) {

	input := &dynamodb.ScanInput{
		TableName: aws.String("Task-Tracker"),
	}
	dbSession := awsConnection()

	allData, err := dbSession.Scan(input)
	if err != nil {
		panic(err)
	}

	if len(allData.Items) == 0 {
		log.Println("Table is empty.")
	} else {
		var myTasks Tasks
		for _, value := range allData.Items {
			task := Task{*value["TaskID"].S, *value["Task Name"].S, *value["Task Details"].S, *value["Completion Date"].S}
			myTasks = append(myTasks, task)
		}

		if currentRuntime == "windows" {
			template := template.Must(template.ParseFiles(`webpage\modifytask.html`))
			template.Execute(w, myTasks)
		} else {
			template := template.Must(template.ParseFiles(`webpage/modifytask.html`))
			template.Execute(w, myTasks)
		}

		if r.Method == "POST" {
			itemInput := &dynamodb.PutItemInput{
				TableName: aws.String("Task-Tracker"),
				Item: map[string]*dynamodb.AttributeValue{
					"TaskID": {
						S: aws.String(r.FormValue("TaskID")),
					},
					"Task Name": {
						S: aws.String(r.FormValue("TaskName")),
					},
					"Task Details": {
						S: aws.String(r.FormValue("TaskDetails")),
					},
					"Completion Date": {
						S: aws.String(r.FormValue("CompleteBy")),
					},
				},
			}
			log.Println(r.FormValue("TaskID"))
			_, err := dbSession.PutItem(itemInput)
			if err != nil {
				panic(err)
			}

		}
	}

}

// Function for serving the delete tasks page. The values are scanned from the DynamoDB table first and populate the relevant fields, the user can then
// select the TaskID which they want to delete.
func deleteTask(w http.ResponseWriter, r *http.Request) {
	taskIDs := make(map[string]string)

	dbSession := awsConnection()

	input := &dynamodb.ScanInput{
		TableName: aws.String("Task-Tracker"),
	}

	allData, err := dbSession.Scan(input)
	if err != nil {
		panic(err)
	}
	for _, value := range allData.Items {
		taskIDs[*value["TaskID"].S] = *value["Task Name"].S
	}

	if currentRuntime == "windows" {
		template := template.Must(template.ParseFiles(`webpage\deletetasks.html`))
		template.Execute(w, taskIDs)
	} else {
		template := template.Must(template.ParseFiles(`webpage/deletetasks.html`))
		template.Execute(w, taskIDs)
	}

	if r.Method == "POST" {
		itemDelete := &dynamodb.DeleteItemInput{
			Key: map[string]*dynamodb.AttributeValue{
				"TaskID": {
					S: aws.String(r.FormValue("TaskID")),
				},
			},
			TableName: aws.String("Task-Tracker"),
		}

		_, err := dbSession.DeleteItem(itemDelete)
		if err != nil {
			panic(err)
		}
	}

}

// Route for serving the main menu.
func menu(w http.ResponseWriter, r *http.Request) {
	if currentRuntime == "windows" {
		template := template.Must(template.ParseFiles(`webpage\menu.html`))
		template.Execute(w, nil)
	} else {
		template := template.Must(template.ParseFiles(`webpage/menu.html`))
		template.Execute(w, nil)
	}

}

func main() {
	log.Println("Server running...")

	myHTTPRouter := mux.NewRouter().StrictSlash(true)
	myHTTPRouter.HandleFunc("/", menu)
	myHTTPRouter.HandleFunc("/View", viewTasks)
	myHTTPRouter.HandleFunc("/Add", addTasks)
	myHTTPRouter.HandleFunc("/Delete", deleteTask)
	myHTTPRouter.HandleFunc("/Modify", modifyTask)

	myHTTPRouter.HandleFunc("/api/Add", addTaskAPI).Methods("POST")
	myHTTPRouter.HandleFunc("/api/ViewAll", viewAllTasksAPI)

	http.ListenAndServe(":8080", myHTTPRouter)
}
