'use client'

import { Download } from 'lucide-react'
import {
  CodeBlock,
  CodeBlockHeader,
  CodeBlockTitle,
  CodeBlockActions,
  CodeBlockCopyButton,
  CodeBlockContent,
} from '@/components/ui/code-block'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  algorithmExamples,
  isBucket,
  settingsError,
  type AlgorithmSettings,
} from './algorithm-examples'

function download(name: string, content: string) {
  const url = URL.createObjectURL(
    new Blob([content], { type: 'text/plain;charset=utf-8' }),
  )
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
function Snippet({ name, value }: { name: string; value: string }) {
  return (
    <CodeBlock size="sm">
      <CodeBlockHeader>
        <CodeBlockTitle>{name}</CodeBlockTitle>
        <CodeBlockActions>
          <CodeBlockCopyButton value={value} />
          <button
            type="button"
            aria-label={`Download ${name}`}
            onClick={() => download(name, value)}
            className="flex size-8 items-center justify-center rounded-md hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
          >
            <Download className="size-3.5" />
          </button>
        </CodeBlockActions>
      </CodeBlockHeader>
      <CodeBlockContent className="max-h-[32rem]" aria-label={name}>
        {value}
      </CodeBlockContent>
    </CodeBlock>
  )
}
export function AlgorithmSetup({
  algorithm,
  mode,
  settings,
  onChange,
}: {
  algorithm: string
  mode: 'docker' | 'sdk'
  settings: AlgorithmSettings
  onChange: (settings: AlgorithmSettings) => void
}) {
  const error = settingsError(settings, algorithm)
  const examples = error ? null : algorithmExamples(algorithm, settings)
  const fields: {
    key: 'limit' | 'windowSeconds' | 'maxKeys' | 'capacity'
    label: string
    max: number
  }[] = [
    {
      key: 'limit',
      label: isBucket(algorithm) ? 'Rate per window' : 'Requests per window',
      max: 1000000,
    },
    { key: 'windowSeconds', label: 'Window (seconds)', max: 3600 },
    { key: 'maxKeys', label: 'Maximum identities', max: 1000000 },
    ...(isBucket(algorithm)
      ? [{ key: 'capacity' as const, label: 'Burst capacity', max: 1000000 }]
      : []),
  ]
  return (
    <section
      id={mode}
      className="min-w-0 scroll-mt-20 space-y-5"
      aria-label={mode === 'docker' ? 'Docker configuration' : 'Go SDK example'}
    >
      <div>
        <h2 className="text-xl font-semibold">
          {mode === 'docker' ? 'Run the Docker image' : 'Use the Go SDK'}
        </h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {mode === 'docker'
            ? 'Download two files. Build the image. Start the selected algorithm.'
            : 'A complete Go example using the same algorithm and settings.'}
        </p>
      </div>
      <fieldset className="rounded-xl border border-border bg-muted/20 p-4">
        <legend className="px-1 text-xs font-medium">
          Shared settings · Docker & Go
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-2 text-xs font-medium">
            Storage
            <select
              aria-label="Example storage"
              value={settings.storage}
              onChange={(event) =>
                onChange({
                  ...settings,
                  storage: event.target.value as AlgorithmSettings['storage'],
                })
              }
              className="rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
            >
              <option value="ram">RAM · one process</option>
              <option value="redis">Redis · shared across replicas</option>
            </select>
          </label>
          {fields.map((field) => (
            <label
              key={field.key}
              className="flex min-w-0 flex-col gap-2 text-xs font-medium"
            >
              {field.label}
              <input
                aria-label={field.label}
                type="number"
                min={1}
                max={field.max}
                step={1}
                value={
                  Number.isNaN(settings[field.key]) ? '' : settings[field.key]
                }
                onChange={(event) =>
                  onChange({
                    ...settings,
                    [field.key]: event.target.valueAsNumber,
                  })
                }
                className="min-w-0 rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
              />
            </label>
          ))}
        </div>
        <p className="mt-3 text-xs leading-5 text-muted-foreground">
          {isBucket(algorithm)
            ? 'Rate / window sets refill or drain speed. Capacity sets the immediate burst.'
            : 'Limit / window sets the request allowance for each IP address.'}{' '}
          Changing settings updates both examples. Restart after changing a
          running config.
        </p>
      </fieldset>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {examples &&
        (mode === 'docker' ? (
          <>
            <p className="text-xs leading-6 text-muted-foreground">
              Save these files beside the repository Dockerfile in{' '}
              <code>disturbuted-rate-limiting/</code>. Builds{' '}
              <code>rate-limiter:local</code>; the included backend stays
              private.
            </p>
            <Tabs defaultValue="config" className="gap-4">
              <div className="min-w-0 overflow-x-auto rounded-lg">
                <TabsList
                  aria-label="Docker files"
                  className="h-10! w-full min-w-max"
                >
                  <TabsTrigger value="config">Proxy JSON</TabsTrigger>
                  <TabsTrigger value="compose">Compose YAML</TabsTrigger>
                  <TabsTrigger value="run">Run commands</TabsTrigger>
                </TabsList>
              </div>
              <TabsContent value="config">
                <Snippet name="proxy.algorithm.json" value={examples.config} />
              </TabsContent>
              <TabsContent value="compose">
                <Snippet
                  name="compose.algorithm.yaml"
                  value={examples.compose}
                />
              </TabsContent>
              <TabsContent value="run">
                <Snippet name="run.sh" value={examples.commands} />
              </TabsContent>
            </Tabs>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() =>
                  download('proxy.algorithm.json', examples.config)
                }
                className="min-h-10 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
              >
                Download proxy JSON
              </button>
              <button
                type="button"
                onClick={() =>
                  download('compose.algorithm.yaml', examples.compose)
                }
                className="min-h-10 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
              >
                Download Compose YAML
              </button>
            </div>
            <p className="text-xs leading-6 text-muted-foreground">
              {settings.storage === 'redis'
                ? 'Redis listens on loopback port 6380 for the Go example. Keep the volume when stopping to preserve state.'
                : 'RAM quota resets when the gateway restarts.'}{' '}
              Port 8088 must be free. The image loads settings at startup.
              Different settings revisions use separate quota scopes.
            </p>
          </>
        ) : (
          <>
            <Snippet name="main.go" value={examples.go} />
            <p className="text-xs leading-6 text-muted-foreground">
              Save as <code>examples/selected-algorithm/main.go</code> inside
              the Go module. Run{' '}
              <code>go run ./examples/selected-algorithm</code>.{' '}
              {settings.storage === 'redis'
                ? 'Start the Redis Docker example first, or set REDIS_URL to your Redis server. Proxy and SDK use separate quota namespaces.'
                : 'For a long-running service, schedule client.CleanupExpired() periodically.'}
            </p>
          </>
        ))}
    </section>
  )
}
