import type { Metadata } from 'next'
import Image from 'next/image'
import { ArrowUpRight, Play } from 'lucide-react'
import { DocsIndex } from '../rate-limiter/_components/DocsIndex'
import content from './about-content.json'

export const metadata: Metadata = {
  title: 'Video Streaming | StreamFlow',
  description:
    'The story behind StreamFlow: adaptive HLS streaming, sprite sheet previews, and the video processing pipeline.',
}

const sections: [string, string][] = content.map(({ id, title }) => [id, title])

export default function VideoStreamingPage() {
  return (
    <div className="mx-auto w-full max-w-6xl min-w-0 px-5 py-8 md:px-10 md:py-12 lg:pr-64">
      <DocsIndex sections={sections} />
      <header className="border-b border-border pb-8">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Play className="size-4" aria-hidden="true" /> Video Streaming
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          StreamFlow
        </h1>
        <p className="mt-4 text-base leading-7 text-muted-foreground">
          Building an OTT platform from curiosity, not just code.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <a
            href="https://streamflow-sigma.vercel.app/"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            Launch StreamFlow{' '}
            <ArrowUpRight className="size-4" aria-hidden="true" />
            <span className="sr-only">(opens in a new tab)</span>
          </a>
          <a
            href="https://streamflow-sigma.vercel.app/about"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
          >
            Original About page{' '}
            <ArrowUpRight className="size-4" aria-hidden="true" />
            <span className="sr-only">(opens in a new tab)</span>
          </a>
        </div>
        <p className="mt-4 text-xs leading-5 text-muted-foreground">
          Content reproduced from StreamFlow’s About page.
        </p>
      </header>
      <article className="mt-10 space-y-10">
        {content.map(({ id, title, blocks }) => (
          <section
            key={id}
            id={id}
            aria-labelledby={`${id}-title`}
            className="scroll-mt-20 space-y-4"
          >
            <h2
              id={`${id}-title`}
              className="text-xl font-semibold tracking-tight"
            >
              {title}
            </h2>
            {blocks.map((block, index) => {
              if (block.type === 'list')
                return (
                  <ul
                    key={index}
                    className="list-disc space-y-2 ps-5 text-sm leading-7 text-muted-foreground"
                  >
                    {block.items?.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                )
              if (block.type === 'h3')
                return (
                  <h3 key={index} className="pt-3 text-base font-medium">
                    {block.text}
                  </h3>
                )
              return (
                <p
                  key={index}
                  className="text-sm leading-7 text-muted-foreground"
                >
                  {block.text}
                </p>
              )
            })}
            {id === 'sprite-sheets' && (
              <figure className="overflow-hidden rounded-xl border border-border">
                <Image
                  src="/streamflow/sprite.jpg"
                  width={1600}
                  height={900}
                  alt="Sprite sheet example: video frames in a grid for timeline previews"
                  className="h-auto w-full"
                />
                <figcaption className="border-t border-border px-4 py-3 text-xs leading-5 text-muted-foreground">
                  Sprite sheet example from StreamFlow.{' '}
                  <a
                    href="https://streamflow-sigma.vercel.app/about"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline underline-offset-4"
                  >
                    Try the interactive timeline preview on the original page.
                  </a>
                </figcaption>
              </figure>
            )}
            {id === 'inspiration' && (
              <a
                href="https://www.youtube.com/watch?v=-JtjQ-OA7XE"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 text-sm text-primary underline underline-offset-4"
              >
                YouTube: Sprite sheets &amp; OTT concepts{' '}
                <ArrowUpRight className="size-4" aria-hidden="true" />
              </a>
            )}
          </section>
        ))}
      </article>
    </div>
  )
}
