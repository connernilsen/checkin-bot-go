package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
  "time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

var API_TOKEN string
const SERVICE_URL = "https://slack.com/api/"
var MAIN_CHANNEL_ID string
var MAIN_CHANNEL_NAME string
var CURRENT_THREAD_ID string
var USER_LIST []string
const BOT_NAME = "c4c_checkin"
var CUSTOM_ADMIN_APPENDIX string
var ADMIN_USERS []string

// type to unmarshal JSON Slack responses into
type SlackResponse struct {
  Ok bool
  Channels []ConversationList
  Members []string
  Channel map[string]string
  Type string
  Challenge string
  Event SlackEvent
  Ts string
  User UserInfo
  Error string
}

// type to contain conversation info
type ConversationList struct {
  Id, Name string
  Is_channel, Is_group, Is_im, Is_member, Is_mpim, Is_private bool
}

// type to contain Slack Event callback info
type SlackEvent struct {
  Type string
  Text string
  User string
}

// type to contain user info
type UserInfo struct {
  Id string
  Real_name string
}

// converts a string map to a JSON string
func StringMapToPostBody(m map[string]string) string {
  if m == nil {
    return ""
  }
  b := new(bytes.Buffer)
	fmt.Fprintf(b, "{")
	for key, value := range m {
		fmt.Fprintf(b, "\"%s\":\"%s\",", key, value)
	}
	if len(m) > 0 {
		b.Truncate(b.Len() - 1)
	}
	fmt.Fprintf(b, "}")
	return b.String()
}

// conversts a string map to a url encoded param map
func StringMapToGetBody(m map[string]string, trail bool) string {
	b := new(bytes.Buffer)
	fmt.Fprintf(b, "?")
	for key, value := range m {
		fmt.Fprintf(b, "%s=%s&", key, value)
	}
	if len(m) > 0 && !trail {
		b.Truncate(b.Len() - 2)
	}
	return b.String()
}

// take a request/response body and parse it into a string
func CaptureResponseBody(r io.ReadCloser) string {
	builder := strings.Builder{}

	for {
		bytes := make([]byte, 256)
		length, err := r.Read(bytes)
		builder.Write(bytes[:length])
		if err != nil {
			break
		}
	}

	r.Close()
	return builder.String()
}

// unmarshal get url-encoded string into a string map
func UnmarshalGet(req string) map[string]string {
  body := make(map[string]string)
  split := strings.Split(req, "&")
  for _, val := range split {
    temp := strings.Split(val, "=")
    body[temp[0]] = temp[1]
  }
  return body
}

// determines if user with given userId is an admin user
func IsAdminUser(userId string) bool {
  for _, id := range ADMIN_USERS {
    if userId == id {
      return true
    }
  }
  return false
}

// handle http responses and error, and convert the response into SlackResponse or error
func HandleResponse(res *http.Response, err error, logBody bool) (resp SlackResponse, retErr error) {
	if err != nil {
		log.Println("Error in HandleResponse:")
		log.Println(err)
    return SlackResponse{}, err
	} else {
		log.Printf("Status: %s", res.Status)
    body := CaptureResponseBody(res.Body)
    if logBody {
		  log.Printf(body)
    }
    var resp SlackResponse
    err = json.Unmarshal([]byte(body), &resp)
    return resp, err
	}
}

// perform request by creating client and using do method
func DoRequest(url string, req *http.Request) (res *http.Response, err error) {
	req.Header.Add("charset", "utf-8")
	client := http.Client{}

	log.Println("Pre-request")
	defer log.Println("Post-request")
	return client.Do(req)
}

// perform HTTP GET request and return the response
func PerformGet(url string, headers map[string]string, body map[string]string, includeAuth bool) (res *http.Response, err error) {
	if includeAuth {
		url = fmt.Sprintf("%s%s%stoken=%s", SERVICE_URL, url, StringMapToGetBody(body, true), API_TOKEN)
	} else {
		url = fmt.Sprint(SERVICE_URL, url, StringMapToGetBody(body, false))
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	log.Printf("Performing GET for URL: %s\n", url)
	return DoRequest(url, req)
}

// perform HTTP POST request and return the response
func PerformPost(url string, headers map[string]string, body map[string]string, includeAuth bool) (res *http.Response, err error) {
	url = fmt.Sprint(SERVICE_URL, url)
	req, err := http.NewRequest("POST", url, strings.NewReader(StringMapToPostBody(body)))
	if err != nil {
		return nil, err
	}

	if includeAuth {
		authHeader := fmt.Sprintf("Bearer %s", API_TOKEN)
		req.Header.Add("Authorization", authHeader)
	}
	req.Header.Add("Content-Type", "application/json")

	for key, value := range headers {
		req.Header.Add(key, value)
	}

  log.Printf("Performing POST for URL: %s with Body:\n", url)
  log.Println(StringMapToPostBody(body))
	return DoRequest(url, req)
}

// send the given message to the given channel and optional thread, then return the resulting SlackResponse
func SendMessage(message, channelId, thread string) (body SlackResponse, err error) {
	params := make(map[string]string)
	params["text"] = message
	if thread != "" {
		params["thread_ts"] = thread
	}
	params["channel"] = channelId

	res, err := PerformPost("chat.postMessage", nil, params, true)

  return HandleResponse(res, err, false)
}

// hit up the Slack test endpoint
func TestSlack(error bool, message string) {
	var params string
	if error {
		fmt.Printf("Error test for %s", message)
		params = fmt.Sprintf("error=%s", message)
	} else {
		fmt.Printf("Testing %s", message)
		params = fmt.Sprintf("test_message=%s", message)
	}
	url := fmt.Sprintf("api.test?%s", params)
	res, err := PerformPost(url, nil, nil, false)

	HandleResponse(res, err, true)
}

// get all (public) channels in the Slack workspace and optionally log the response, 
// then return a map of names to ConversationList
// if MAIN_CHANNEL_ID is not set, then it is updated 
func GetChannels(logAnswer bool) (channels map[string]ConversationList) {
	url := "conversations.list"
	res, err := PerformGet(url, nil, nil, true)
  body, err := HandleResponse(res, err, false)

	if err != nil || !body.Ok {
		log.Println("Error in GetChannels:")
		log.Println(err)
    log.Printf("body.Ok: %t\n", body.Ok)
    return nil
	}

  channels = make(map[string]ConversationList)
  for _, item := range body.Channels {
    channels[item.Name] = item
  }
  if logAnswer {
    log.Println(channels)
  }

  if MAIN_CHANNEL_ID == "" {
    MAIN_CHANNEL_ID = channels[MAIN_CHANNEL_NAME].Id
    log.Printf("Main Channel Name: %s, Id: %s\n", MAIN_CHANNEL_NAME, MAIN_CHANNEL_ID)
  }
  return channels
}

// get all users in the given channel and optionally log the response, then return the list of userIds
func GetUsers(channelId string, logAnswer bool) (users []string) {
  url := "conversations.members"
  params := make(map[string]string)
  params["channel"] = channelId
  res, err := PerformGet(url, nil, params, true)
  body, err := HandleResponse(res, err, false)

  if err != nil || !body.Ok {
    log.Println("Error in GetUsers:")
    log.Println(err)
    log.Printf("body.Ok: %t\n", body.Ok)
    log.Printf("response body error: %s\n", body.Error)
    return nil
  }

  if logAnswer {
    log.Println(body.Members)
  }

  return body.Members
}

// send the given message to the given user by userId
func MessageUser(userId, message string) {
  url := "conversations.open"
  params := make(map[string]string)
  params["users"] = userId
  res, err := PerformPost(url, nil, params, true)
  body, err := HandleResponse(res, err, false)

  if err != nil || !body.Ok {
    log.Println("Error in MessageUser:")
    log.Println(err)
    log.Printf("body.Ok: %t\n", body.Ok)
    return 
  }
  newChannelId := body.Channel["id"]

  SendMessage(message, newChannelId, "")
}

// get the username of a userId
func GetUsername(userId string) (name string, err error) {
  url := "users.info"
  params := make(map[string]string)
  params["user"] = userId
  res, err := PerformGet(url, nil, params, true)

  body, err := HandleResponse(res, err, false)
  if err != nil || !body.Ok {
    log.Println("Error in MessageUser:")
    log.Println(err)
    log.Printf("body.Ok: %t\n", body.Ok)
    return "", err
  }

  return body.User.Real_name, err
}

// update the global list of users by setting the user to an empty string if they should be
// removed (i.e. if they have already responded)
// returns a boolean representing whether or not the user has been updated
func UpdateUserList(userId string) bool {
  for pos, id := range USER_LIST {
    if userId == id {
      USER_LIST[pos] = ""
      return true
    }
  }
  return false
}

// the handler for the /test endpoint
func TestSuccess(w http.ResponseWriter, r *http.Request) {
	TestSlack(false, r.URL.Path)
	w.Write([]byte("Tested Success"))
}

// the handler for the /testError endpoint
func TestError(w http.ResponseWriter, r *http.Request) {
	TestSlack(true, r.URL.Path)
	w.Write([]byte("Tested Error"))
}

// the handler for the /close endpoint
// if the given user_id is not part of the admin users global var or empty,
// then the function does not proceed
func CloseCheckin(w http.ResponseWriter, r *http.Request) {
  req := CaptureResponseBody(r.Body)
  reqBody := UnmarshalGet(req)
  userId := reqBody["user_id"]
  if !IsAdminUser(userId) && userId != "" {
    w.Write([]byte("You are not an admin"))
    return
  }
  SendMessage("Checkin is now closed", MAIN_CHANNEL_ID, CURRENT_THREAD_ID)
  CURRENT_THREAD_ID = ""
  w.Write([]byte(fmt.Sprintf("Checkin Closed%s", CUSTOM_ADMIN_APPENDIX)))
}

// log global vars to console
func LogVars(w http.ResponseWriter, r *http.Request) {
  log.Println("API_TOKEN: ")
  log.Println(API_TOKEN)
  log.Println("SERVICE_URL")
  log.Println(SERVICE_URL)
  log.Println("MAIN_CHANNEL_NAME")
  log.Println(MAIN_CHANNEL_NAME)
  log.Println("MAIN_CHANNEL_ID")
  log.Println(MAIN_CHANNEL_ID)
  log.Println("CURRENT_THREAD_ID")
  log.Println(CURRENT_THREAD_ID)
  log.Println("USER_LIST")
  log.Println(USER_LIST)
  log.Println("BOT_NAME")
  log.Println(BOT_NAME)
  w.Write([]byte("Done"))
}

// handle / endpoint callback
// if type is 'url_verification', then returns verificaiton token
// if type is 'event_callback', event type is 'message', and initiator is not the bot, then 
// handle user message response
func HandleCallback(w http.ResponseWriter, r *http.Request) {
  req := CaptureResponseBody(r.Body)
  var body SlackResponse
  json.Unmarshal([]byte(req), &body)
  if body.Type == "url_verification" {
    w.Write([]byte(body.Challenge))
    log.Println("Slack API Callback Url Verified")
    return
  } else if body.Type == "event_callback" && body.Event.Type == "message" {
    w.Write([]byte("Message Received"))
    name, err := GetUsername(body.Event.User)
    if name == BOT_NAME {
      return
    }
    log.Printf("Handle Message Callback for user: %s\n", body.Event.User)
    if CURRENT_THREAD_ID == "" {
      MessageUser(body.Event.User, "There is currently no open checkin session")
      return
    }

    if !UpdateUserList(body.Event.User) {
      MessageUser(body.Event.User, "Cannot change body once sent, please go to thread and post followup.")
      return
    }

    if err != nil {
      log.Println("Error in HandleCallback:")
      log.Println(err)
    }
    log.Printf("%s's Response: %s", name, body.Event.Text)
    messageResp, err := SendMessage(fmt.Sprintf("%s's Response: %s", name, body.Event.Text), MAIN_CHANNEL_ID, CURRENT_THREAD_ID)
    log.Println(messageResp.Error)
  } else {

    log.Println("Unknown callback:")
    log.Println(req)
    w.Write([]byte("HandleCallback but no valid condition found"))
  }
}

// handles the checkin initiation endpoint
// updates the MAIN_CHANNEL_ID global var, gets the users in the main channel, 
// and notifies them about the checkin
// if the given user_id is not part of the admin users global var or empty,
// then the function does not proceed
func HandleCheckin(w http.ResponseWriter, r *http.Request) {
  req := CaptureResponseBody(r.Body)
  reqBody := UnmarshalGet(req)
  userId := reqBody["user_id"]
  if !IsAdminUser(userId) && userId != "" {
    w.Write([]byte("You are not an admin"))
    return
  }

  if MAIN_CHANNEL_ID == "" {
    GetChannels(false)
  }

  USER_LIST = GetUsers(MAIN_CHANNEL_ID, false)
  for _, userId := range USER_LIST {
    MessageUser(userId, "Hey! It's time for your checkin. Let me know what you're gonna do, how long you think it will take, and when you plan on working on this -- *in one message please*. Thanks.")
  }

  body, err := SendMessage(fmt.Sprintf("Here are the results for the standup on `%s`", time.Now().Format("Jan 2")), MAIN_CHANNEL_ID, "")
  if err != nil {
    log.Println("Error in HandleCheckin")
  }

  CURRENT_THREAD_ID = body.Ts
  w.Write([]byte(fmt.Sprintf("Checkin Sent%s", CUSTOM_ADMIN_APPENDIX)))
}

// reminds the users who have not yet completed their checkin that they need to complete it
// if the given user_id is not part of the admin users global var or empty,
// then the function does not proceed
func RemindAwaiting(w http.ResponseWriter, r *http.Request) {
  if CURRENT_THREAD_ID == "" {
    w.Write([]byte("There is currently no open checkin session, try again later ;)"))
  }

  for _, userId := range USER_LIST {
    if userId != "" {
      MessageUser(userId, "Don't forget to complete the checkin session")
    }
  }

  w.Write([]byte(fmt.Sprintf("Users have been notified%s", CUSTOM_ADMIN_APPENDIX)))
}

func main() {
  // sets up necessary env vars
  var port string
  err := godotenv.Load()
  if err != nil {
    log.Println(err)
  }
  if os.Getenv("ENVIRONMENT") == "development" {
	  port = os.Getenv("PORT")
  } else {
    port = fmt.Sprintf(":%s", os.Getenv("PORT"))
  }
	API_TOKEN = os.Getenv("API_TOKEN")
  MAIN_CHANNEL_ID = os.Getenv("MAIN_CHANNEL_ID")
  MAIN_CHANNEL_NAME = os.Getenv("MAIN_CHANNEL_NAME")
  ADMIN_USERS = strings.Split(os.Getenv("ADMIN_USERS"), ",")
  CUSTOM_ADMIN_APPENDIX = os.Getenv("CUSTOM_ADMIN_APPENDIX")
  if port == "" || port == ":" || API_TOKEN == "" || MAIN_CHANNEL_NAME == "" {
		log.Fatal("PORT, MAIN_CHANNEL_NAME, and API_TOKEN must be set")
	}

  // sets up router
  log.Printf("Server starting on Port: %s...\n", port)
	router := mux.NewRouter()

  // setup routes
	router.HandleFunc("/", HandleCallback)
	router.HandleFunc("/test", TestSuccess)
	router.HandleFunc("/testError", TestError)
  router.HandleFunc("/getVars", LogVars)
  router.HandleFunc("/checkin", HandleCheckin)
  router.HandleFunc("/remind", RemindAwaiting)
  router.HandleFunc("/close", CloseCheckin)
	log.Fatal(http.ListenAndServe(port, router))
}
