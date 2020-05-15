package ocrworker

import (
	"math/rand"
)

func randomMOTD() string {

	names := [...]string{
		`<p>Nice day to put slinkies on an escalator!</p>
		<pre>__   ; '.'  :
|  +. :  ;   :   __
.-+\   ':  ;   ,,-'  :
'.  '.  '\ ;  :'   ,'
,-+'+. '. \.;.:  _,' '-.
'.__  "-.::||::.:'___-+'
-"  '--..::::::_____.IIHb
'-:______.:; '::,,...,;:HB\
.+         \ ::,,...,;:HB \
'-.______.:+ \'+.,...,;:P'  \
.-'           \              \
'-.______.:+   \______________\
.-::::::::,     BBBBBBBBBBBBBBB
::,,...,;:HB    BBBBBBBBBBBBBBB
::,,...,;:HB    BBBBBBBBBBBBBBB
::,,...,;:HB\   BBBBBBBBBBBBBBB
::,,...,;:HB \  BBBBBBBBBBBBBBB
'+.,...,;:P'  \ BBBBBBBBBBBBBBB
               \BBBBBBBBBBBBBBB</pre>`,
		"Oh, marmalade!",
		"There it is, the near-death star.",
		"Hooray, I'm useful. I'm having a wonderful time.",
		"Oh, Lord, I'm on the verge of a nervous melt-down.",
		"The guy who wrote this is gone",
		"It's running everywhere!",
		"No comments, no documentation but 24 tickets",
		"00 0110011 1000011 010011 01000 0110 10011 11 001010 001 0001011 110010",
		"Robot Nite - Designated device drivers drink free",
		"When I'm in command, every mission is a suicide-mission.",
		"Protecting the environment is a crime",
		"I choose to believe what I was programmed to believe.",
		"No fair! You changed the outcome by measuring it.",
		"I can't keep running people over. I'm not famous enough to get away with it",
		"Creating tomorrow's legacy today!",
		"Help! A guinea pig tricked me!",
		"You win again, gravity!",
		"Good news! There's a report on TV with some very bad news.",
		"Who was that guy? -Fry Your momma! Now shut up and drag me to work. -Bender",
		"Oh no! I should do something....but i am already in my pajamas.",
		"I'd tell you the joke about UDP, but you might not get it.",
		`<p>Here be dragons!</p>
		<pre>
     ,-------,  ,  ,   ,-------,
      )  ,' /(  |\/|   )\ ',  (
       )'  /  \ (qp)  /  \  '(
        ) /___ \_\/(_/ ___\ (
         '    '-(   )-'    '
         )w^w(
         (W_W)
         ((
         ))
         ((
         ) 
             </pre>`,
	}
	number := rand.Intn(len(names))

	return names[number]
}

// GenerateLandingPage will generate a simple landing page
func GenerateLandingPage(appStop bool, technicalError bool) string {
	statusArray := [4]string{}

	if technicalError {
		statusArray[0] = `<a class="nes-btn is-error" href="">ERROR</a>`
		statusArray[1] = `<a class="nes-btn is-disabled" href="">RUNNING</a>`
		statusArray[2] = `<a class="nes-btn is-disabled" href="">WARNING</a>`
		statusArray[3] = `Manual intervention required!`
	} else {
		switch appStop {
		case true:
			statusArray[0] = `<a class="nes-btn is-warning" href="">WARNING</a>`
			statusArray[1] = `<a class="nes-btn is-disabled" href="">RUNNING</a>`
			statusArray[2] = `<a class="nes-btn is-disabled" href="">ERROR</a>`
			statusArray[3] = `Someone is shutting me down!`
		case false:
			statusArray[0] = `<a class="nes-btn is-success" href="">RUNNING</a>`
			statusArray[1] = `<a class="nes-btn is-disabled" href="">WARNING</a>`
			statusArray[2] = `<a class="nes-btn is-disabled" href="">ERROR</a>`
			statusArray[3] = `I'm fine!`
		}
	}

	head := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>open-ocr</title><style> html, body{font-family: "Fixedsys,Courier,monospace";}
body {cursor: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAABFklEQVRYR9WXURLDIAhE6/0PbSdOtUpcd1Gnpv1KGpTHBpCE1/cXq+vrMph7dGvXZTtpfW10DCA5jrH1H0Jhs5E0hnZdCR+vb5S8Nn8mQCeS9BdSalYJqMBjAGzq59xAESN7VFVUgV8AZB/dZBR7QTFDCqGquvUBVVoEtgIwpQRzmANSFHgWQKExHdIrPeuMvQNDarXe6nC/AutgV3JW+6bgqQLeV8FekRtgV+ToDKEKnACYKsfZjjkam7a0ZpYTytwmgainpC3HvwBocgKOxqRjehoR9DFKNFYtOwCGYCszobeCbl26N6yyQ6g8X/Wex/rBPsNEV6qAMaJPMynIHQCoSqS9JSMmwef51LflTgCRszU7DvAGiV6mHWfsaVUAAAAASUVORK5CYII=),auto;}
body{font-family: "Press Start 2P";}
@font-face {
  font-family: "Press Start 2P";
  src: url(data:application/fontwoff2;charset=utf-8;base64,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) format('woff2')}
body {max-width: 960px; min-width: 320px;margin: 0 auto;}section {margin-top: 3em; margin: 3em 1.5em 0 1.5em;}section:last-of-type {margin-bottom: 3em;}li {margin-top: 0.8em;}.nes-container {position: relative; padding: 1.5rem 2rem; border-color: #000; border-style: solid;border-width: 4px;} .nes-container.with-title > .title {display: table;padding: 0 .5rem;margin: -2.2rem 0 1rem; font-size:1rem;background-color: #fff;}

.nes-btn {border-image-slice: 2;border-image-width: 2;border-image-outset: 2;text-align: center; position: relative;
border-style: solid;border-width: 4px;text-decoration: none;background-color: #92cc41;display: inline-block;padding: 6px 8px;color: #fff; width: 268px;
border-image-source: url('data:image/svg+xml;utf8,<?xml version="1.0" encoding="UTF-8" ?><svg version="1.1" width="5" height="5" xmlns="http://www.w3.org/2000/svg"><path d="M2 1 h1 v1 h-1 z M1 2 h1 v1 h-1 z M3 2 h1 v1 h-1 z M2 3 h1 v1 h-1 z" fill="rgb(33,37,41)" /></svg>');}

.nes-btn.is-warning{background-color: #f2c409}
.nes-btn.is-error{background-color: #ce372b}
.nes-btn.is-disabled, .nes-btn.is-disabled:focus, .nes-btn.is-disabled:hover {
    color: #212529;
    background-color: #d3d3d3;
    opacity: .6;
}

.nes-balloon.from-left::after {
    bottom: -18px;
    width: 18px;
    height: 4px;
    margin-right: 8px;
    color: #212529;
    background-color: #fff;
    box-shadow: -4px 0,4px 0,-4px 4px #fff,0 4px,-8px 4px,-4px 8px,-8px 8px;
}
.nes-balloon.from-left::after, .nes-balloon.from-left::before {
    left: 2rem;
}
.nes-balloon::after, .nes-balloon::before {
    position: absolute;
    content: "";
}
.nes-balloon.from-left::before {
    bottom: -14px;
    width: 26px;
    height: 10px;
    background-color: #fff;
    border-right: 4px solid #212529;
    border-left: 4px solid #212529;
}
.message-list > .message > .nes-balloon {
    max-width: 550px;
}
.nes-balloon {
    border-image-slice: 3;
    border-image-width: 3;
    border-image-repeat: stretch;
    border-image-source: url('data:image/svg+xml;utf8,<?xml version="1.0" encoding="UTF-8" ?><svg version="1.1" width="8" height="8" xmlns="http://www.w3.org/2000/svg"><path d="M3 1 h1 v1 h-1 z M4 1 h1 v1 h-1 z M2 2 h1 v1 h-1 z M5 2 h1 v1 h-1 z M1 3 h1 v1 h-1 z M6 3 h1 v1 h-1 z M1 4 h1 v1 h-1 z M6 4 h1 v1 h-1 z M2 5 h1 v1 h-1 z M5 5 h1 v1 h-1 z M3 6 h1 v1 h-1 z M4 6 h1 v1 h-1 z" fill="rgb(33,37,41)" /></svg>');
    border-image-outset: 2;
    position: relative;
    display: inline-block;
    padding: 1rem 1.5rem;
    margin: 8px;
	margin-bottom: 8px;
    margin-bottom: 30px;
    background-color: #fff;
	border-image-repeat: stretch;
}
.nes-balloon, .nes-btn, .nes-container.is-rounded, .nes-container.is-rounded.is-dark, .nes-dialog.is-rounded, .nes-dialog.is-rounded.is-dark, .nes-input, .nes-progress, .nes-progress.is-rounded, .nes-select select, .nes-table.is-bordered, .nes-table.is-dark.is-bordered, .nes-textarea {
    border-style: solid;
    border-width: 4px;
}
*, ::after, ::before {
    box-sizing: border-box;
}
.nes-container.is-centered {
    text-align: center;
}
.nes-container {
    position: relative;
    padding: 1.5rem 2rem;
    border-color: #000;
    border-style: solid;
    border-width: 4px;
    margin-bottom: 1.5rem;
    margin-top: 1.5em;
}
pre {
    font-size: 8px;
    font-family: "Press Start 2P";

}
</style></head><body><section class="nes-container with-title">	<h2 class="title">Open-ocr  ></h2>
  <div class="nes-balloon from-left">
      <p>` + statusArray[3] + `</p>
    </div>
<div>`
	buttons := statusArray[0] + "\n" + statusArray[1] + "\n" + statusArray[2]
	middle := `</div><div class="nes-container with-title is-centered"> <p class="title">MOTD</p>`
	MOTD := randomMOTD()
	tail := `</div><div class="nes-container with-title">
<p class="title">info</p>
<div class="lists">
  <ul class="nes-list is-circle">
    <li><p>Need <a href="https://godoc.org/github.com/xf0e/open-ocr">docs</a>?</p></li>
    <li><p> <a href="https://github.com/xf0e/open-ocr">repository</a></p></li>
	<li><p>written as DDD process (russian: Davai Davai Deploy)</p></li>
  </ul>
</div>
</section></body> </html>`

	return head + buttons + middle + MOTD + tail

}
