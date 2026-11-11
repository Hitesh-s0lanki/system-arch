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
  Database,
  Globe,
  Network,
  Server,
  ShieldCheck,
  ShieldX,
} from 'lucide-react'
import '@xyflow/react/dist/style.css'

type Mode = 'gateway' | 'distributed'
type Step = Node<
  {
    title: string
    description: string
    kind: 'client' | 'router' | 'service' | 'limiter' | 'storage' | 'rejected'
    badge?: string
  },
  'step'
>

const icons = {
  client: Globe,
  router: Network,
  service: Server,
  limiter: ShieldCheck,
  storage: Database,
  rejected: ShieldX,
}

function StepNode({ data }: NodeProps<Step>) {
  const Icon = icons[data.kind]
  return (
    <div
      className={`w-[190px] rounded-xl border bg-card p-4 shadow-sm ${data.kind === 'limiter' || data.kind === 'storage' ? 'border-primary/50' : data.kind === 'rejected' ? 'border-destructive/40' : 'border-border'}`}
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
      {data.badge && (
        <div className="mt-3 rounded-md bg-primary/10 px-2 py-1.5 text-xs font-medium text-primary">
          {data.badge}
        </div>
      )}
      {['client', 'router', 'limiter'].includes(data.kind) && (
        <Handle type="source" position={Position.Right} />
      )}
    </div>
  )
}

const nodeTypes = { step: StepNode }
function step(
  id: string,
  x: number,
  y: number,
  title: string,
  description: string,
  kind: Step['data']['kind'],
  badge?: string,
): Step {
  return {
    id,
    type: 'step',
    position: { x, y },
    data: { title, description, kind, badge },
  }
}
function connect(
  source: string,
  target: string,
  options: Partial<Edge> = {},
): Edge {
  return { id: `${source}-${target}`, source, target, ...options }
}
const primaryEdge = {
  style: { stroke: 'var(--primary)', strokeWidth: 2 },
  markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--primary)' },
}
const diagrams = {
  gateway: {
    nodes: [
      step('client', 0, 210, 'Client', 'Incoming HTTP request', 'client'),
      step(
        'gateway',
        310,
        180,
        'API gateway',
        'After TLS and authentication, check quota before routing',
        'limiter',
        'Central rate limiter',
      ),
      step('auth', 640, 0, 'Auth', 'Sign-in and user sessions', 'service'),
      step(
        'ads',
        640,
        155,
        'Ads Library',
        'Ad search and creative assets',
        'service',
      ),
      step(
        'keywords',
        640,
        310,
        'Keyword Planner',
        'Keyword research and planning',
        'service',
      ),
      step(
        'rejected',
        640,
        465,
        'Return to client',
        '429 + Retry-After when the shared quota is exhausted',
        'rejected',
      ),
    ],
    edges: [
      connect('client', 'gateway', { label: 'HTTP request' }),
      connect('gateway', 'auth'),
      connect('gateway', 'ads'),
      connect('gateway', 'keywords'),
      connect('gateway', 'rejected', {
        label: 'Over limit',
        style: { stroke: 'var(--destructive)', strokeWidth: 2 },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: 'var(--destructive)',
        },
      }),
    ],
    caption:
      'The gateway enforces a shared caller quota before routing to Auth, Ads Library, or Keyword Planner. Rate limiting lives inside the gateway; the services receive allowed requests.',
  },
  distributed: {
    nodes: [
      step('client', 0, 200, 'Client', 'Incoming HTTP request', 'client'),
      step(
        'balancer',
        270,
        190,
        'Load balancer',
        'Routes traffic across healthy replicas',
        'router',
      ),
      step(
        'replica-1',
        540,
        0,
        'Limiter replica 1',
        'Stateless gateway or proxy',
        'limiter',
      ),
      step(
        'replica-2',
        540,
        190,
        'Limiter replica 2',
        'Stateless gateway or proxy',
        'limiter',
      ),
      step(
        'replica-3',
        540,
        380,
        'Limiter replica 3',
        'Stateless gateway or proxy',
        'limiter',
      ),
      step(
        'backend',
        860,
        80,
        'Backend services',
        'Receive allowed requests from any replica',
        'service',
      ),
      step(
        'redis',
        860,
        420,
        'Shared Redis',
        'Atomic quota checks across every replica',
        'storage',
        'One quota per caller',
      ),
    ],
    edges: [
      connect('client', 'balancer'),
      ...['replica-1', 'replica-2', 'replica-3'].flatMap((id) => [
        connect('balancer', id),
        connect(id, 'backend', {
          label: id === 'replica-1' ? 'Allowed' : undefined,
        }),
        connect(id, 'redis', {
          ...primaryEdge,
          style: { ...primaryEdge.style, strokeDasharray: '5 5' },
          markerStart: {
            type: MarkerType.ArrowClosed,
            color: 'var(--primary)',
          },
        }),
      ]),
    ],
    caption:
      'Solid lines carry requests from the client through the load balancer and a limiter replica to the backend. Dashed lines show atomic checks against shared Redis. Each replica returns 429 on limit or 503 on storage failure.',
  },
}

const edgeOptions = {
  type: 'smoothstep',
  markerEnd: { type: MarkerType.ArrowClosed },
  style: { stroke: 'var(--muted-foreground)', strokeWidth: 1.5 },
  labelStyle: { fill: 'var(--foreground)', fontSize: 12 },
  labelBgStyle: { fill: 'var(--card)' },
  labelBgPadding: [7, 4] as [number, number],
  labelBgBorderRadius: 5,
}

export function ModeFlowDiagram({ mode }: { mode: Mode }) {
  const diagram = diagrams[mode]
  return (
    <figure className="overflow-hidden rounded-xl border border-border bg-muted/20">
      <div className="h-[400px] w-full sm:h-[530px]">
        <ReactFlow
          aria-label={`${mode === 'gateway' ? 'Gateway' : 'Distributed'} mode request flow`}
          defaultNodes={diagram.nodes}
          defaultEdges={diagram.edges}
          nodeTypes={nodeTypes}
          defaultEdgeOptions={edgeOptions}
          fitView
          fitViewOptions={{ padding: 0.15 }}
          minZoom={0.2}
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
        {diagram.caption} Pan or use the zoom controls to explore the flow.
      </figcaption>
    </figure>
  )
}
