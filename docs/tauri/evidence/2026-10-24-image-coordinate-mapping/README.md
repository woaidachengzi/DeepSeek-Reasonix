# Chrome screenshot coordinate mismatch

Date: 2026-10-24. Analyzed installed version: 0.1.54.

The saved observation reported a Chrome window of 1686x983 logical points,
while the actual attached JPEG header reported 1920x1119 pixels. The model
requested a window-point click at (800,351), using the screenshot's first-row
pixel coordinates. The result reported `window_server_mouse`; the subsequent
conversation reported the following email row opening instead. The original
capture enlarged smaller windows up to 1920 pixels, making this mistake easy.
No private screenshot, email body, account identifier or session ID is included
in this evidence directory.

Correct conversion of that screenshot point is:

- x = 800 * 1686 / 1920 = 702.5 window points.
- y = 351 * 983 / 1119 = about 308.34 window points.

The pointer gateway already creates move/down/up events. Adding another mouse
implementation or artificial human-like delays does not correct this coordinate
mismatch.

## Fix and automated scope

The production capture-size helper never upscales a normal window. A 1686x983
window now requests a 1686x983 image. The longest image side is still capped at
1920; large or portrait windows can be downscaled.

For screenshot clicks and coordinate fills, `coordinate_space=image` requests
host conversion from the exact verified JPEG dimensions. `image_geometry`
reports those dimensions and points per pixel. The host does not accept
caller-supplied image dimensions or add screen origins. Omitted coordinate
space or explicit `window` retains the previous window-point contract.

Controller tests replay the observed 1920x1119-to-1686x983 mapping, 1:1 captures,
large-window downscaling, click/fill actions, positive and negative window
origins, unknown geometry, out-of-image coordinates, expiry and moved windows.
Refused mapping consumes the old receipt and sends no input. Caller coordinate
variables are not mutated. Compact diagnostic export retains mapping numbers.

The native event-envelope runner compiles the same production capture-size
helper and checks normal, small, 4K, portrait and invalid sizes. Its pointer
callback inspects event objects without posting real input. No new system
permissions were requested or changed for this work.

These checks establish coordinate conversion and guard behavior, not live
Chrome/Gmail reply or send acceptance. No real email was opened, replied to or
sent during this work. The new package must still be tested in the user's
intended browser task.
