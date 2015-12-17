# mumble-jukebox

A simple mumble bot for playing audio from YouTube, and [other sites supported by youtube-dl](https://rg3.github.io/youtube-dl/supportedsites.html). Simply send the bot a link, and it will download and play the audio. This is pretty limited at the moment, but will be extended over time. Feel free to report bugs and suggest features via the [issue-tracker](https://github.com/joshheinrichs/mumble-player/issues).

### Setup

Requires [FFmpeg](https://www.ffmpeg.org/) and [youtube-dl](https://rg3.github.io/youtube-dl/) to work properly.

You'll also need to create a config file named `config.yaml` with at least a username and address to connect to a server. An example file is shown below:

```yaml
username: "foo"
address: "example.com:64738"
```

The config file is parsed into [gumble.Config](https://godoc.org/github.com/layeh/gumble/gumble#Config), so you can add other fields from that struct if you want to overwrite default values, such as adding a password.
