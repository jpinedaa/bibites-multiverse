# Third-party notices

The Apache License 2.0 applies to original Bibites Multiverse work in this repository.
It does not replace the licenses or copyright terms of third-party material.

Any release archive that contains a component below must also contain its applicable notice.

## The Bibites

*The Bibites*, its game files, and its assets are not part of the Apache-licensed work.
They remain subject to the terms of their copyright owners.

Some authorized release editions can contain a game payload. Its separate license file governs that payload.

## BepInEx 5.4.23.3

Bibites Multiverse uses and can redistribute the upstream BepInEx 5.4.23.3 package without modification.
That version uses the MIT License. See the [upstream source](https://github.com/BepInEx/BepInEx/tree/v5.4.23.3).

> MIT License
>
> Copyright (c) 2018 Bepis
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.

The upstream package also contains UnityDoorstop, HarmonyX, MonoMod, and Mono.Cecil.
The [BepInEx 5.4.23.3 source page](https://github.com/BepInEx/BepInEx/tree/v5.4.23.3) identifies their versions and source repositories.
Those components retain their own terms.

## MediaMTX 1.20.0

The hosted live-stream origin downloads MediaMTX 1.20.0 from the upstream
release. That version uses the MIT License. See the
[upstream source](https://github.com/bluenviron/mediamtx/tree/v1.20.0).

> MIT License
>
> Copyright (c) 2019 aler9
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.

## coder/websocket 1.8.15

The Go binaries link to `github.com/coder/websocket` version 1.8.15.
That version uses the ISC License. See the [upstream source](https://github.com/coder/websocket/tree/v1.8.15).

> Copyright (c) 2025 Coder
>
> Permission to use, copy, modify, and distribute this software for any
> purpose with or without fee is hereby granted, provided that the above
> copyright notice and this permission notice appear in all copies.
>
> THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
> WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
> MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
> ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
> WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
> ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
> OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

## lxn/walk and lxn/win

The Windows launcher's window links `github.com/lxn/walk`
version 0.0.0-20210112085537-c389da54e794 and `github.com/lxn/win`
version 0.0.0-20210218163916-a377121e959e, which draw it with the operating
system's own controls. Both use a 3-clause BSD license. See the upstream sources
for [walk](https://github.com/lxn/walk/tree/c389da54e794) and
[win](https://github.com/lxn/win/tree/a377121e959e).

> Copyright (c) 2010 The Walk Authors. All rights reserved.
> Copyright (c) 2010 The win Authors. All rights reserved.
>
> Redistribution and use in source and binary forms, with or without
> modification, are permitted provided that the following conditions
> are met:
> 1. Redistributions of source code must retain the above copyright
>    notice, this list of conditions and the following disclaimer.
> 2. Redistributions in binary form must reproduce the above copyright
>    notice, this list of conditions and the following disclaimer in the
>    documentation and/or other materials provided with the distribution.
> 3. The names of the authors may not be used to endorse or promote products
>    derived from this software without specific prior written permission.
>
> THIS SOFTWARE IS PROVIDED BY THE AUTHORS ``AS IS'' AND ANY EXPRESS OR
> IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
> OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
> IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY DIRECT, INDIRECT,
> INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
> NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
> DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
> THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
> (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
> THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

That window also links two libraries those two bring with them:
`gopkg.in/Knetic/govaluate.v3` version 3.0.0, under the MIT License
(Copyright (c) 2014-2016 George Lester,
[upstream source](https://github.com/Knetic/govaluate/tree/v3.0.0)), and
`golang.org/x/sys` version 0.0.0-20201018230417-eeed37f84f13, under the same
3-clause BSD license as the Go standard library
(Copyright (c) 2009 The Go Authors,
[upstream source](https://cs.opensource.google/go/x/sys)). The full texts of both
are in those sources, and each is reproduced verbatim under
`go/go.sum`'s recorded module contents.

## rsrc 0.10.2 (build tool; nothing of it ships)

`github.com/akavel/rsrc` version 0.10.2 compiles the Windows launcher's resource
object at build time - the Common Controls 6 manifest and the application icon,
both of which are this project's own files. No rsrc code is linked into any
shipped binary. It uses the MIT License
(Copyright (c) 2013-2017 The rsrc Authors,
[upstream source](https://github.com/akavel/rsrc/tree/v0.10.2)).
