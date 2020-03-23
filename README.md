# A Slack bot for standups and check-ins 
To run the bot, perform the following steps:
- Create a `.env` file in the root directory with the following values:
  - `API_TOKEN` - set to your Slack API *user* token
  - `MAIN_CHANNEL_NAME` - set to the channel you want the aggregated responses to be sent in
  - `PORT` - set to the port you want this to run on (must be prefixed with a `:`, ex `:8000`)
  - `ADMIN_USERS` - sets the list of admin users by userId, separated by `,`
  - `MAIN_CHANNEL_ID` (optional) - if you want to override the channel id and ignore the channel name
  - `CUSTOM_ADMIN_APPENDIX` (optional) - something to be appended at the end of responses to admin commands
  - `ENVIRONMENT` (optional) - set to `development` if you want this to be run in development
- Compile with `go build main.go` and run with `./main`

## Slack Bot Setup
### Slash Commands
Set up the following slash commands:
- `/checkin`, for the `/checkin` bot endpoint
- `/remindcheckin`, for the `/remind` bot endpoint
- `/endcheckin`, for the `/close` bot endpoint

### Event Subscriptions
Turned on, with the `/` endpoint set as the Request URL.
Don't forget to subscribe to the `message.im` bot event.

### OAuth & Permissions
Set the following scopes for OAuth:
- `channels:read`
- `chat:write`
- `commands`
- `groups:read`
- `im:history`
- `im:read`
- `im:write`
- `mpim:read`
- `users:read`

## Commands
The current endpoints are:
- `/` - handles the Slack Event Subscription callbacks
- `/test` - hits up the test endpoint of Slack's API
- `/testError` - hits up the test endpoint above but should return an error
- `/getVars` - prints out current global variables to the log
- `/checkin` - handles the slash callback for `/checkin`
- `/remind` - handles the slash callback for `/remindcheckin`
- `/close` - handles the slash callback for `/endcheckin`

