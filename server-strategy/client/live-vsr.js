class VideoSpeechRecognition {

  constructor (media, options) {
    let defaults = {
      mode               : VideoSpeechRecognition.Mode.Live,

      // todo: actually utilize this
      aheadPlayheadLimit : 30,  // seconds
      behindPlayheadLimit: 30
    }

    this._options           = Object.assign({}, defaults, options)
    this._started           = false
    this._media             = media
    this._textTracks        = {}
  }

  static get Mode() {
    return {
      Live: 0,
      VOD : 1   // Will not prune buffer if used. Potential memory problems tho?
    }
  }

  start () {
    let videoEl = this._media
    let tracks = this._textTracks

    if (!Object.keys(tracks).length) {
      tracks.subtitles = videoEl.addTextTrack('subtitles', 'English (auto-generated)', 'en')
      tracks.metadata = videoEl.addTextTrack('metadata')
    }

    Object.values(tracks).forEach(track => track.mode = 'hidden')

    this._started = true
  }

  stop () {
    if (this._started) return

    this._cleanTracks()
    this._started = false
  }

  processTranscriptionForRange (transcription, range) {
    this._translateTranscriptToCues(transcription, range)
    console.info(`
      [VSR] processTranscriptionForRange | translated and added cues from transcript for time range
      start: ${range.start} | end: ${range.end}
    `)

    // if (this._options.mode === VideoSpeechRecognition.Mode.Live) {
    //   console.info('[VSR] processTranscriptionForRange | live mode, pruning old cues')
    //   this._pruneOldCues()
    // }
  }

  _translateTranscriptToCues (recognitionResponse, range) {
    if (recognitionResponse && !recognitionResponse.hasOwnProperty('results')) return

    // Flattening down to a single dimensional array of transcript objects
    let transcripts = recognitionResponse.results
      .map(report => report.alternatives)
      .reduce((acc, val) => acc.concat(val), [])

    const VTTCue = window.VTTCue || window.TextTrackCue

    // TODO: more intelligent grouping, should be done by the
    //       tiny delta between start and end times of nanosecond precision

    let numPerGroup = 10
    let cueGroups = []

    transcripts.forEach(transcript => {
      this._processConfidenceScore(transcript.confidence, range)

      let cues = transcript.words
        .map(structuredWord => {
          let start = range.start + this._fromStructuredNanoTime(structuredWord.start_time)
          let end = range.start + this._fromStructuredNanoTime(structuredWord.end_time)

          return { start, end, word: structuredWord.word }
        })

      let i = 0
      let cuesCount = cues.length
      let expectedNumOfGroups = Math.floor( cuesCount / numPerGroup )
      let group = []

      do {
        let cue = cues.shift()
        let next = cues[0]

        if (cue)
          group.push(cue)

        let isLast = ( cue && !next )

        if (!cue || isLast || group.length === numPerGroup) {
          let groupClone = group.slice(0)
          cueGroups.push(groupClone)
          group.length = 0
        }

        if (!cue)
          break

        i++

      } while (i < cuesCount)
    })

    // Averaging out the start/end times to group words together, as nanosecond accuracy causes words to appear 1-by-1 very quickly
    cueGroups.forEach(group => {
      let firstCueStartTime = group[0].start
      let lastCueEndTime = group[ group.length - 1 ].end

      let contents = group
        .map(cue => cue.word)
        .join(' ')

      let combinedCue = new VTTCue(firstCueStartTime, lastCueEndTime, contents)
      this._textTracks.subtitles.addCue(combinedCue)
    })
  }

  _processConfidenceScore (score, range) {
    // Creating a metadata cue for this range
    const VTTCue = window.VTTCue || window.TextTrackCue

    let cue = new VTTCue(range.start, range.end, JSON.stringify({
      confidence: score,
      range
    }))

    this._textTracks.metadata.addCue(cue)
  }

  _fromStructuredNanoTime (struct) {
    let seconds = struct.seconds
    let nanos = struct.nanos

    let result = 0

    if (seconds)
      result = seconds

    if (nanos)
      // e.g. Translating 4000000 nanos -> 0.4 seconds
      result += ( ( nanos / 1000000 ) / 1000 )

    return result
  }

  /**
   * Removes cues that are behind the ( playhead - behindLimit )
   */
  _pruneOldCues () {
    // (behind/ahead): ~
    // playhead      : *
    // buffered      : +
    // past threshold: -
    //
    //   |-----|+++++++++++++++*++++++++++++++++++++++++++++++++++++++++++++++
    //   |     |~~~~~~~~~~~~~~~*~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
    //      ^ Target range to remove cues in
    //
    // TODO
  }

  _cleanTracks () {
    Object
      .values(this._textTracks)
      .forEach(track => {
        do {
          track.removeCue(track.cues[0])
        } while (typeof track.cues[0] !== 'undefined')

        track.mode = 'hidden'
      })
  }

}
