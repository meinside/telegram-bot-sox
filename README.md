# telegram-bot-sox

A Telegram bot which receives voice messages, converts them with [sox](http://sox.sourceforge.net/), and returns them back.

Inspired by [this project](http://planet-geek.com/2015/10/29/hacks/using-a-raspberry-pi-as-a-realtime-voice-changer-for-halloween/).

## 0. Prepare

### macOS

```bash
$ brew install sox --with-lame --with-flac --with-libvorbis --with-opusfile
```

### Linux

```bash
# TODO
# XXX - sox from the package manager doesn't come with opus support...
```

## 1. Build

```bash
$ go get -d github.com/meinside/telegram-bot-sox
$ cd $GOPATH/src/github.com/meinside/telegram-bot-sox
$ go build
```

## 2. Configure

```bash
$ cp config.json.sample config.json
$ vi config.json
```

## 3. Run

Just run it:

```bash
$ ./telegram-bot-sox
```

### Run as a service

If you run it as a service,

#### macOS (launchd)

Copy sample .plist file:

```bash
$ sudo cp service/telegram-bot-sox.plist /Library/LaunchDaemons/telegram-bot-sox.plist
```

and edit values:

```
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>telegram-bot-sox</string>
	<key>ProgramArguments</key>
	<array>
		<string>/path/to/telegram-bot-sox/telegram-bot-sox</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
```

Now load it with:

```bash
$ sudo launchctl load /Library/LaunchDaemons/telegram-bot-sox.plist
```

#### Linux (systemd)

```bash
$ sudo cp service/telegram-bot-sox.service /lib/systemd/system/
$ sudo vi /lib/systemd/system/telegram-bot-sox.service
```

and edit **User**, **Group**, **WorkingDirectory** and **ExecStart** values.

It will launch automatically on boot with:

```bash
$ sudo systemctl enable telegram-bot-sox.service
```

and will start with:

```bash
$ sudo systemctl start telegram-bot-sox.service
```

## License

MIT

