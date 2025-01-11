# <div align="center">asciiplayer</align>

<div align="center">A video player for your terminal</div>

# 🎉 Features

- 🎬 Play most video formats (uses ffmpeg under the hood)
- 🔊 Supports audio playback
- 📐 Automatically resize to the terminals current size
- 🔤 Different character sets
- 🎨 ${{\large\textsf{{\color{Red}C}{\color{Orange}o}{\color{Yellow}l}{\color{Green}o}{\color{Aqua}r} {\color{Purple}s}{\color{Pink}u}{\color{Red}p}{\color{Orange}p}{\color{Yellow}o}{\color{Green}r}{\color{Aqua}t}}}}\$

# Demo

#### ASCII video

<video src="https://github.com/user-attachments/assets/eb0c0cdb-6712-447b-a8a9-debe6915ee7c"></video>
(sorry for the stuttering and bad audio quality, my computer couldn't handle the recording)

#### Color support

![An image from Big Buck Bunny renderd on a terminal](https://github.com/user-attachments/assets/55e37c60-093d-4c54-b126-b5bbc23ebdd3)

# Usage

#### Play a video:

```sh
asciiplayer video.mp4
asciiplayer animated.gif # also supports gif
asciiplayer video.mkv # ... and many other formats
```

#### More flags:

```sh
asciiplayer -c video.mp4 # enable color
asciiplayer -c -ch filled video.mp4 # use unicode full blocks (█) to render colored video
asciiplayer -fps 10 video.mp4 # play video at specific fps
asciiplayer -height 20 video.mp4 # play video at a specific resolution
asciiplayer -h # show help
```

# Download

Get the binary from the [releases tab](https://github.com/Ecasept/asciiplayer/releases).

You can also build the binary yourself:

```sh
git clone https://github.com/Ecasept/asciiplayer
cd asciiplayer
go build -o build/asciiplayer .
./build/asciiplayer -h # verify that it worked
```
