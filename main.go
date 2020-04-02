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
var OPEN_CHECKIN_STR string
var CLOSE_CHECKIN_STR string
var REMIND_CHECKIN_STR string

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

// maps a list of userIds to list of usernames
func MapIdsToNames(strs []string) []string {
  for pos, val := range strs {
    if val != "" {
      name, _ := GetUsername(val)
      if name != BOT_NAME {
        strs[pos] = name
      } else {
        strs[pos] = ""
      }
    }
  }
  return strs
}

// flattens a list of strings into a string
func FlattenList(strs []string) string {
  builder := strings.Builder{}
  for pos, val := range strs {
    if val != "" {
      if pos != 0 {
        builder.WriteString(", ")
      }
      builder.WriteString(val)
    }
  }
  return builder.String()
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
func CloseCheckinHandler(w http.ResponseWriter, r *http.Request) {
  req := CaptureResponseBody(r.Body)
  reqBody := UnmarshalGet(req)
  userId := reqBody["user_id"]
  if !IsAdminUser(userId) && userId != "" {
    w.Write([]byte("You are not an admin"))
    return
  }
  CloseCheckin()
  w.Write([]byte(fmt.Sprintf("Checkin Closed%s", CUSTOM_ADMIN_APPENDIX)))
}

func CloseCheckin() {
  uncompletedUsers := FlattenList(MapIdsToNames(USER_LIST))
  var uncompletedMessage string
  if uncompletedUsers == "" {
    uncompletedMessage = ""
  } else {
    uncompletedMessage = fmt.Sprintf(" These users did not complete the checkin: %s", uncompletedUsers)
  }
  SendMessage(fmt.Sprintf("Checkin is now closed.%s", uncompletedMessage), MAIN_CHANNEL_ID, CURRENT_THREAD_ID)
  CURRENT_THREAD_ID = ""
}

// Opens checkin by getting the main channel id, notifying users, opening the 
// main thread message in the MAIN_CHANNEL_NAME, and saving the thread id
func OpenCheckin() {
  if MAIN_CHANNEL_ID == "" {
    GetChannels(false)
  }

  USER_LIST = GetUsers(MAIN_CHANNEL_ID, false)
  for _, userId := range USER_LIST {
    MessageUser(userId, "Hey! It's time for your checkin. Let me know what you're gonna do, how long you think it will take, and when you plan on working on this -- *in one message please*. Thanks :)")
  }

  loc, _ := time.LoadLocation("America/New_York")
  time := time.Now().In(loc).Format("Jan 2, 2006 at 3:04pm")
  body, err := SendMessage(fmt.Sprintf("Here are the results for the standup on `%s`", time), MAIN_CHANNEL_ID, "")
  if err != nil {
    log.Println("Error in HandleCheckin")
  }

  CURRENT_THREAD_ID = body.Ts
}

// Reminds users who have not completed checkin to complete checkin
func RemindCheckin() {
  for _, userId := range USER_LIST {
    if userId != "" {
      MessageUser(userId, "Don't forget to complete the checkin session!")
    }
  }
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
  log.Println("OPEN_CHECKIN_STR")
  log.Println(OPEN_CHECKIN_STR)
  log.Println("CLOSE_CHECKIN_STR")
  log.Println(CLOSE_CHECKIN_STR)
  w.Write([]byte("Done"))
}

// handle / endpoint callback
// if type is 'url_verification', then returns verificaiton token
// if type is 'event_callback', event type is 'message', and initiator is not the bot, then 
// handle user message response
// if type is 'event_callback' and event type is 'app_mention', then open or close depending on text
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
      MessageUser(body.Event.User, "There is currently no open checkin session. Please try again later.")
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
    MessageUser(body.Event.User, fmt.Sprintf("Hey, thanks for your response! You should soon see it in <#%s> under the most recent thread. Hope the rest of your day goes well ;)", MAIN_CHANNEL_ID))
    log.Printf("%s's Response: %s", name, body.Event.Text)
    messageResp, err := SendMessage(fmt.Sprintf("%s's Response: %s", name, body.Event.Text), MAIN_CHANNEL_ID, CURRENT_THREAD_ID)
    log.Println(messageResp.Error)
  } else if body.Type == "event_callback" && body.Event.Type == "app_mention" {
    if strings.Contains(body.Event.Text, OPEN_CHECKIN_STR) {
      OpenCheckin()
      log.Println("Checkin Opened by Event Callback")
    } else if strings.Contains(body.Event.Text, CLOSE_CHECKIN_STR) {
      CloseCheckin()
      log.Println("Checkin Closed by Event Callback")
    } else if strings.Contains(body.Event.Text, REMIND_CHECKIN_STR) { 
      RemindCheckin()
      log.Println("Remind Awaiting by Event Callback")
    } else {
      log.Println("No action performed in Event Callback")
    }
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

  OpenCheckin()

  w.Write([]byte(fmt.Sprintf("Checkin Sent%s", CUSTOM_ADMIN_APPENDIX)))
}

// reminds the users who have not yet completed their checkin that they need to complete it
// if the given user_id is not part of the admin users global var or empty,
// then the function does not proceed
func RemindAwaiting(w http.ResponseWriter, r *http.Request) {
  if CURRENT_THREAD_ID == "" {
    w.Write([]byte("There is currently no open checkin session, try again later ;)"))
  }

  RemindCheckin()
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
  OPEN_CHECKIN_STR = os.Getenv("OPEN_CHECKIN_STR")
  CLOSE_CHECKIN_STR = os.Getenv("CLOSE_CHECKIN_STR")
  REMIND_CHECKIN_STR = os.Getenv("REMIND_CHECKIN_STR")
  if port == "" || port == ":" || API_TOKEN == "" || MAIN_CHANNEL_NAME == "" {
		log.Fatal("PORT, MAIN_CHANNEL_NAME, and API_TOKEN must be set")
	}

  if OPEN_CHECKIN_STR == CLOSE_CHECKIN_STR {
    log.Println("OPEN_CHECKIN_STR and CLOSE_CHECKIN_STR are the same, cannot open or close checkin using reminders")
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
  router.HandleFunc("/close", CloseCheckinHandler)
	log.Fatal(http.ListenAndServe(port, router))
}
