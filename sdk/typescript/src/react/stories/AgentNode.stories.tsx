/**
 * AgentNode.stories.tsx — AgentNode 组件 Storybook stories。
 */

import type { Meta, StoryObj } from '@storybook/react';
import { CollaborationAgentNode, STATUS_LABEL } from '../collaboration/AgentNode.js';
import type { CollabAgentStatus } from '../collaboration/CollaborationView.js';

const meta = {
  title: 'Collaboration/AgentNode',
  component: CollaborationAgentNode,
  tags: ['autodocs'],
} satisfies Meta<typeof CollaborationAgentNode>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Idle: Story = {
  args: {
    agent: { id: 'a1', name: 'Research Agent', status: 'idle' },
  },
};

export const Thinking: Story = {
  args: {
    agent: { id: 'a2', name: 'Analysis Agent', status: 'thinking', activity: 'Processing data' },
  },
};

export const Working: Story = {
  args: {
    agent: { id: 'a3', name: 'Writer Agent', status: 'working', role: 'Content Creator' },
  },
};

export const Waiting: Story = {
  args: {
    agent: { id: 'a4', name: 'Review Agent', status: 'waiting' },
  },
};

export const Error: Story = {
  args: {
    agent: { id: 'a5', name: 'Failed Agent', status: 'error' },
  },
};

export const Done: Story = {
  args: {
    agent: { id: 'a6', name: 'Completed Agent', status: 'done' },
  },
};

export const AllStatuses: Story = {
  render: () => {
    const statuses: CollabAgentStatus[] = ['idle', 'thinking', 'working', 'waiting', 'error', 'done'];
    return (
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        {statuses.map((s) => (
          <CollaborationAgentNode
            key={s}
            agent={{ id: s, name: `${STATUS_LABEL[s]} Agent`, status: s }}
          />
        ))}
      </div>
    );
  },
};
