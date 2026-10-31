// navigator.vibrate works on Android over plain HTTP; iOS has no web haptics.
// Always guarded: some browsers throw if called before a user gesture.
export function hapticTick(ms = 8): void {
  try {
    if (typeof navigator !== 'undefined' && typeof navigator.vibrate === 'function') navigator.vibrate(ms)
  } catch {
    // ignore
  }
}
