package ocrworker

// GenerateLandingPage will generate a simple landing page
func GenerateLandingPage() string {

	text := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>open-ocr</title>` +
		`<style> html, body{font-family: "Fixedsys,Courier,monospace";}body {max-width: 960px; min-width: 320px;` +
		`margin: 0 auto;}section {margin-top: 3em; margin: 3em 1.5em 0 1.5em;}section:last-of-type {margin-bottom: 3em;}` +
		`li {margin-top: 0.8em;}.nes-container {position: relative; padding: 1.5rem 2rem; border-color: #000; border-style: solid;` +
		`border-width: 4px;} .nes-container.with-title > .title {display: table;padding: 0 .5rem;margin: -2.2rem 0 1rem; font-size:` +
		`1rem;background-color: #fff;}` +
		`.nes-btn {border-image-slice: 2;border-image-width: 2;border-image-outset: 2;position: relative;border-style: solid;border-width: 4px;` +
		`text-decoration: none;background-color: #92cc41;display: inline-block;padding: 6px 8px;color: #fff;` +
		`border-image-source: url('data:image/svg+xml;utf8,<?xml version="1.0" encoding="UTF-8" ?><svg version="1.1" width="5" height="5" xmlns="http://www.w3.org/2000/svg"><path d="M2 1 h1 v1 h-1 z M1 2 h1 v1 h-1 z M3 2 h1 v1 h-1 z M2 3 h1 v1 h-1 z" fill="rgb(33,37,41)" /></svg>');}` +
		`</style></head><body>` +
		`<section class="nes-container with-title">	<h2 class="title">Open-ocr  ></h2>` +
		`<div><a class="nes-btn is-success" href="">RUNNING</a>` +
		`<pre>  __   ; '.'  :
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
               \BBBBBBBBBBBBBBB</pre><p>Nice day to put slinkies on an escalator!</p></div><p>proudly made with QBASIC:)</p>
			<p>Need <a href="https://godoc.org/github.com/xf0e/open-ocr">docs</a>?</p></section></body> </html>`
	return text

}
