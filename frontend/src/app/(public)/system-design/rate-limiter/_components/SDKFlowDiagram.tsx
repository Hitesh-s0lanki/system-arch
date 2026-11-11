'use client'

import {
  Background,
  Controls,
  Handle,
  MarkerType,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import {
  Code2,
  Globe,
  KeyRound,
  LayoutGrid,
  Network,
  Search,
} from 'lucide-react'
import '@xyflow/react/dist/style.css'

type ServiceNode = Node<
  {
    title: string
    description: string
    kind: 'client' | 'gateway' | 'auth' | 'ads' | 'keywords'
    sdk?: boolean
  },
  'service'
>

const icons = {
  client: Globe,
  gateway: Network,
  auth: KeyRound,
  ads: LayoutGrid,
  keywords: Search,
}

function ServiceStep({ data }: NodeProps<ServiceNode>) {
  const Icon = icons[data.kind]
  return (
    <div
      className={`w-[190px] rounded-xl border bg-card p-4 shadow-sm ${data.sdk ? 'border-primary/50' : 'border-border'}`}
    >
      {data.kind !== 'client' && (
        <Handle type="target" position={Position.Left} />
      )}
      <div className="flex items-center gap-2 text-foreground">
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        <span className="text-sm font-semibold">{data.title}</span>
      </div>
      <p className="mt-2 text-xs leading-5 text-muted-foreground">
        {data.description}
      </p>
      {data.sdk && (
        <div className="mt-3 flex items-center gap-1.5 rounded-md bg-primary/10 px-2 py-1.5 text-xs font-medium text-primary">
          <Code2 className="size-3.5 shrink-0" aria-hidden="true" />
          Rate limiter SDK
        </div>
      )}
      {(data.kind === 'client' || data.kind === 'gateway') && (
        <Handle type="source" position={Position.Right} />
      )}
    </div>
  )
}

const nodeTypes = { service: ServiceStep }
const nodes: ServiceNode[] = [
  {
    id: 'client',
    type: 'service',
    position: { x: 0, y: 180 },
    data: {
      title: 'Client',
      description: 'Incoming HTTP request',
      kind: 'client',
    },
  },
  {
    id: 'gateway',
    type: 'service',
    position: { x: 310, y: 170 },
    data: {
      title: 'API gateway',
      description: 'Routes requests to the destination service',
      kind: 'gateway',
    },
  },
  {
    id: 'auth',
    type: 'service',
    position: { x: 620, y: 0 },
    data: {
      title: 'Auth',
      description: 'Sign-in and user sessions',
      kind: 'auth',
    },
  },
  {
    id: 'ads',
    type: 'service',
    position: { x: 620, y: 170 },
    data: {
      title: 'Ads Library',
      description: 'Ad search and creative assets',
      kind: 'ads',
    },
  },
  {
    id: 'keywords',
    type: 'service',
    position: { x: 620, y: 340 },
    data: {
      title: 'Keyword Planner',
      description: 'Enforces its own request quota in process',
      kind: 'keywords',
      sdk: true,
    },
  },
]

const edges: Edge[] = [
  { id: 'request', source: 'client', target: 'gateway', label: 'HTTP request' },
  { id: 'auth', source: 'gateway', target: 'auth' },
  { id: 'ads', source: 'gateway', target: 'ads' },
  {
    id: 'keywords',
    source: 'gateway',
    target: 'keywords',
    style: { stroke: 'var(--primary)', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--primary)' },
  },
]

const edgeOptions = {
  type: 'smoothstep',
  markerEnd: { type: MarkerType.ArrowClosed },
  style: { stroke: 'var(--muted-foreground)', strokeWidth: 1.5 },
  labelStyle: { fill: 'var(--foreground)', fontSize: 12 },
  labelBgStyle: { fill: 'var(--card)' },
  labelBgPadding: [7, 4] as [number, number],
  labelBgBorderRadius: 5,
}

export function SDKFlowDiagram() {
  return (
    <figure className="overflow-hidden rounded-xl border border-border bg-muted/20">
      <div className="h-[400px] w-full sm:h-[530px]">
        <ReactFlow
          aria-label="SDK mode request flow"
          defaultNodes={nodes}
          defaultEdges={edges}
          nodeTypes={nodeTypes}
          defaultEdgeOptions={edgeOptions}
          fitView
          fitViewOptions={{ padding: 0.15 }}
          minZoom={0.3}
          maxZoom={1.5}
          nodesDraggable={false}
          nodesConnectable={false}
          edgesReconnectable={false}
          deleteKeyCode={null}
          zoomOnScroll={false}
          preventScrolling={false}
          colorMode="system"
        >
          <Background color="var(--border)" gap={20} />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
      <figcaption className="border-t border-border px-4 py-3 text-xs leading-5 text-muted-foreground">
        The gateway routes requests to each service. Here, Keyword Planner
        embeds the rate limiter SDK and checks its quota inside the service
        process. Pan or use the zoom controls to explore the flow.
      </figcaption>
    </figure>
  )
}
