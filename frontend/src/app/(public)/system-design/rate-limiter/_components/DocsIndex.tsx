'use client'

import { useEffect, useState } from 'react'

type DocsIndexProps = {
  sections: readonly (readonly [id: string, label: string])[]
}

export function DocsIndex({ sections }: DocsIndexProps) {
  const [activeId, setActiveId] = useState<string | undefined>(sections[0]?.[0])

  useEffect(() => {
    const targets = sections.flatMap(([id]) => {
      const element = document.getElementById(id)
      return element ? [{ id, element }] : []
    })
    let scrollRoot = targets[0]?.element.parentElement
    while (
      scrollRoot &&
      (!/(auto|scroll)/.test(getComputedStyle(scrollRoot).overflowY) ||
        scrollRoot.scrollHeight <= scrollRoot.clientHeight)
    ) {
      scrollRoot = scrollRoot.parentElement
    }
    const scroller = scrollRoot ?? document.scrollingElement
    let frame = 0

    const update = () => {
      frame = 0
      let current: string | undefined = targets[0]?.id
      for (const { id, element } of targets) {
        if (element.getBoundingClientRect().top <= 144) current = id
      }
      if (
        scroller &&
        scroller.scrollTop > 0 &&
        scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 2
      ) {
        const anchor = targets.find(
          ({ id }) => `#${id}` === window.location.hash,
        )
        const bounds = anchor?.element.getBoundingClientRect()
        current =
          bounds && bounds.top >= 44 && bounds.top < window.innerHeight
            ? anchor?.id
            : targets.at(-1)?.id
      }
      setActiveId(current)
    }
    const scheduleUpdate = () => {
      if (!frame) frame = requestAnimationFrame(update)
    }

    scheduleUpdate()
    document.addEventListener('scroll', scheduleUpdate, {
      capture: true,
      passive: true,
    })
    window.addEventListener('resize', scheduleUpdate)
    window.addEventListener('hashchange', scheduleUpdate)
    return () => {
      cancelAnimationFrame(frame)
      document.removeEventListener('scroll', scheduleUpdate, true)
      window.removeEventListener('resize', scheduleUpdate)
      window.removeEventListener('hashchange', scheduleUpdate)
    }
  }, [sections])

  return (
    <aside className="fixed top-24 right-6 bottom-8 hidden w-44 overflow-y-auto lg:block">
      <nav aria-label="On this page" className="space-y-3 text-xs">
        <p className="pl-4 font-medium">On this page</p>
        <div className="border-l border-border">
          {sections.map(([id, label]) => (
            <a
              key={id}
              href={`#${id}`}
              aria-current={activeId === id ? 'location' : undefined}
              onClick={() => setActiveId(id)}
              className={`-ml-px block border-l-2 py-2 pr-2 pl-4 leading-5 transition-colors focus-visible:outline-2 focus-visible:outline-ring ${activeId === id ? 'border-primary bg-primary/5 font-medium text-primary' : 'border-transparent text-muted-foreground hover:text-foreground'}`}
            >
              {label}
            </a>
          ))}
        </div>
      </nav>
    </aside>
  )
}
