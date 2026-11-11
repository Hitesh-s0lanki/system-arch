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
import { Database, Globe, Server, ShieldCheck, ShieldX } from 'lucide-react'
import '@xyflow/react/dist/style.css'

type FlowNode = Node<
  {
    title: string
    description: string
    kind: 'client' | 'proxy' | 'storage' | 'backend' | 'rejected'
  },
  'step'
>

const icons = {
  client: Globe,
  proxy: ShieldCheck,
  storage: Database,
  backend: Server,
  rejected: ShieldX,
}

function StepNode({ data }: NodeProps<FlowNode>) {
  const Icon = icons[data.kind]
  const tone =
    data.kind === 'backend'
      ? 'border-emerald-600/40 text-emerald-700 dark:text-emerald-400'
      : data.kind === 'rejected'
        ? 'border-destructive/40 text-destructive'
        : data.kind === 'proxy'
          ? 'border-primary/50 text-primary'
          : 'border-border text-foreground'

  return (
    <div
      className={`w-[190px] rounded-xl border bg-card p-4 shadow-sm ${tone}`}
    >
      <Handle type="target" position={Position.Top} id="top" />
      <Handle type="target" position={Position.Left} id="left" />
      <div className="mb-2 flex items-center gap-2">
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        <span className="text-sm font-semibold">{data.title}</span>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        {data.description}
      </p>
      <Handle type="source" position={Position.Bottom} id="bottom" />
      <Handle type="source" position={Position.Right} id="right" />
    </div>
  )
}

const nodeTypes = { step: StepNode }
const nodes: FlowNode[] = [
  {
    id: 'client',
    type: 'step',
    position: { x: 0, y: 190 },
    data: {
      title: 'Client',
      description: 'Incoming HTTP request',
      kind: 'client',
    },
  },
  {
    id: 'proxy',
    type: 'step',
    position: { x: 310, y: 170 },
    data: {
      title: 'Rate limiter proxy',
      description: 'Match route · resolve identity · check allowance',
      kind: 'proxy',
    },
  },
  {
    id: 'storage',
    type: 'step',
    position: { x: 310, y: 420 },
    data: {
      title: 'RAM or Redis',
      description: 'Local counters or a shared quota across replicas',
      kind: 'storage',
    },
  },
  {
    id: 'backend',
    type: 'step',
    position: { x: 640, y: 80 },
    data: {
      title: 'Backend service',
      description: 'Receives the original method, headers, and body',
      kind: 'backend',
    },
  },
  {
    id: 'rejected',
    type: 'step',
    position: { x: 640, y: 300 },
    data: {
      title: 'Return to client',
      description: '429 + Retry-After on limit · 503 on storage failure',
      kind: 'rejected',
    },
  },
]

const edges: Edge[] = [
  {
    id: 'request',
    source: 'client',
    target: 'proxy',
    sourceHandle: 'right',
    targetHandle: 'left',
    label: 'HTTP request',
  },
  {
    id: 'counter',
    source: 'proxy',
    target: 'storage',
    sourceHandle: 'bottom',
    targetHandle: 'top',
    label: 'Atomic check',
    markerStart: { type: MarkerType.ArrowClosed },
  },
  {
    id: 'allowed',
    source: 'proxy',
    target: 'backend',
    sourceHandle: 'right',
    targetHandle: 'left',
    label: 'Allowed',
    style: { stroke: 'var(--color-emerald-600)', strokeWidth: 2 },
    markerEnd: {
      type: MarkerType.ArrowClosed,
      color: 'var(--color-emerald-600)',
    },
  },
  {
    id: 'rejected',
    source: 'proxy',
    target: 'rejected',
    sourceHandle: 'right',
    targetHandle: 'left',
    label: 'Limit / failure',
    style: { stroke: 'var(--destructive)', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--destructive)' },
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

export function ProxyFlowDiagram() {
  return (
    <figure className="overflow-hidden rounded-xl border border-border bg-muted/20">
      <div className="h-[440px] w-full sm:h-[530px]">
        <ReactFlow
          aria-label="Proxy mode request flow"
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
        Every request reaches the proxy first. Allowed requests go to the
        backend; rejected requests return directly to the client. Pan or use the
        zoom controls to explore the flow.
      </figcaption>
    </figure>
  )
}
