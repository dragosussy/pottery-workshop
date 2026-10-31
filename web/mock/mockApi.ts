// Dev-only mock API (`npm run dev:mock`). A Vite dev-server middleware over
// the in-memory backend in src/mocks/. It is loaded with a dynamic import from
// vite.config.ts only in `serve` + `mock` mode, so it never reaches a build.
//
//   /__mock                     list scenarios, switch, inject failures
//   /__mock/scenario/<name>     reset to a scenario and go to /
//   /__mock/fail/<mode>         make the NEXT write fail:
//                                 drop-after  (saved, but the response is lost)
//                                 drop-before (connection fails, nothing saved)
//                                 http500     (an error response)
//   MOCK_SCENARIO=ready_now npm run dev:mock   start in a scenario

import type { IncomingMessage, ServerResponse } from 'node:http'
import type { Plugin } from 'vite'
import { createMockBackend } from '../src/mocks/backend'
import { SCENARIOS, SCENARIO_NOTES, type ScenarioName } from '../src/mocks/fixtures'

type FailMode = 'drop-after' | 'drop-before' | 'http500'

const LATENCY_MS = 120
const PING_MS = 20_000
const READINGS_MS = 5_000

function readBody(req: IncomingMessage): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = []
    req.on('data', (c: Buffer) => chunks.push(c))
    req.on('end', () => resolve(Buffer.concat(chunks)))
    req.on('error', reject)
  })
}

export function mockApi(): Plugin {
  const initial = (process.env.MOCK_SCENARIO as ScenarioName | undefined) ?? 'calibrated'
  const backend = createMockBackend(SCENARIOS.includes(initial) ? initial : 'calibrated')
  const clients = new Set<ServerResponse>()
  let failNext: FailMode | null = null
  let changedTimer: ReturnType<typeof setTimeout> | null = null
  let pendingScopes = new Set<string>()
  let pendingIds = new Set<number>()

  const send = (res: ServerResponse, event: string, data: unknown) => {
    res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
  }

  backend.subscribe((scopes, ids) => {
    scopes.forEach((s) => pendingScopes.add(s))
    ids.forEach((i) => pendingIds.add(i))
    if (changedTimer) return
    changedTimer = setTimeout(() => {
      changedTimer = null
      const msg = { scopes: [...pendingScopes], piece_ids: [...pendingIds] }
      pendingScopes = new Set()
      pendingIds = new Set()
      clients.forEach((c) => send(c, 'changed', msg))
    }, 500)
  })

  setInterval(() => {
    if (!clients.size) return
    const r = backend.readingsEvent()
    clients.forEach((c) => send(c, 'readings', r))
  }, READINGS_MS).unref()
  setInterval(() => clients.forEach((c) => c.write(': ping\n\n')), PING_MS).unref()

  function stream(req: IncomingMessage, res: ServerResponse) {
    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-store',
      'X-Accel-Buffering': 'no',
      Connection: 'keep-alive',
    })
    res.write('retry: 5000\n\n')
    send(res, 'hello', backend.hello())
    send(res, 'readings', backend.readingsEvent())
    clients.add(res)
    req.on('close', () => clients.delete(res))
  }

  function indexPage(res: ServerResponse) {
    const rows = SCENARIOS.map(
      (s) => `<li><a href="/__mock/scenario/${s}">${s}</a>${s === backend.scenario ? ' <b>(current)</b>' : ''} — ${SCENARIO_NOTES[s]}</li>`,
    ).join('')
    res.setHeader('Content-Type', 'text/html; charset=utf-8')
    res.end(`<!doctype html><meta name="viewport" content="width=device-width"><title>Mock API</title>
<body style="font:16px system-ui;padding:16px;line-height:1.8">
<h1>Mock API</h1><p>Scenarios (reset the in-memory data):</p><ul>${rows}</ul>
<p>Make the next write fail: <a href="/__mock/fail/drop-after">response lost after saving</a> ·
<a href="/__mock/fail/drop-before">connection fails</a> · <a href="/__mock/fail/http500">HTTP 500</a>
${failNext ? ` — <b>armed: ${failNext}</b>` : ''}</p>
<p><a href="/">Open the app</a> · <a href="/dev/slider">Slider test</a></p></body>`)
  }

  return {
    name: 'humidity-mock-api',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://mock.local')
        const path = url.pathname

        if (path === '/__mock' || path === '/__mock/') return indexPage(res)
        const sc = /^\/__mock\/scenario\/([a-z_]+)$/.exec(path)
        if (sc) {
          const name = sc[1] as ScenarioName
          if (SCENARIOS.includes(name)) backend.reset(name)
          res.statusCode = 302
          res.setHeader('Location', '/')
          return res.end()
        }
        const fl = /^\/__mock\/fail\/([a-z0-9-]+)$/.exec(path)
        if (fl) {
          failNext = (['drop-after', 'drop-before', 'http500'] as const).find((m) => m === fl[1]) ?? null
          res.statusCode = 302
          res.setHeader('Location', '/__mock')
          return res.end()
        }

        if (!path.startsWith('/api/')) return next()
        if (!path.startsWith('/api/v1/')) {
          res.statusCode = 404
          res.setHeader('Content-Type', 'application/json; charset=utf-8')
          return res.end(JSON.stringify({ error: { code: 'not_found', message: 'Unknown route' } }))
        }
        const apiPath = path.slice('/api/v1'.length)
        if (apiPath === '/stream' && req.method === 'GET') return stream(req, res)

        const method = req.method ?? 'GET'
        const raw = method === 'GET' || method === 'HEAD' ? Buffer.alloc(0) : await readBody(req)
        const contentType = req.headers['content-type'] ?? ''
        let json: unknown
        if (raw.length && contentType.includes('application/json')) {
          try {
            json = JSON.parse(raw.toString('utf8'))
          } catch {
            res.statusCode = 400
            res.setHeader('Content-Type', 'application/json; charset=utf-8')
            return res.end(JSON.stringify({ error: { code: 'bad_request', message: 'Malformed JSON' } }))
          }
        }

        const isWrite = method !== 'GET'
        const mode = isWrite ? failNext : null
        if (mode) failNext = null
        if (mode === 'drop-before') return req.socket.destroy()

        await new Promise((r) => setTimeout(r, LATENCY_MS))
        if (mode === 'http500') {
          res.statusCode = 500
          res.setHeader('Content-Type', 'application/json; charset=utf-8')
          return res.end(JSON.stringify({ error: { code: 'internal', message: 'Injected failure' } }))
        }
        const out = backend.handle({ method, path: apiPath, query: url.searchParams, json, bytes: new Uint8Array(raw), contentType })
        if (mode === 'drop-after') return req.socket.destroy() // the write happened; the response is lost
        res.statusCode = out.status
        for (const [k, v] of Object.entries(out.headers)) res.setHeader(k, v)
        res.end(out.body ?? undefined)
      })
    },
  }
}
