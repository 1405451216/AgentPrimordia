/**
 * CollaborationView.stories.tsx — CollaborationView 组件 Storybook stories。
 */

import type { Meta, StoryObj } from '@storybook/react';
import { CollaborationView } from '../collaboration/CollaborationView.js';
import type { CollaborationSession } from '../collaboration/CollaborationView.js';

const meta = {
  title: 'Collaboration/CollaborationView',
  component: CollaborationView,
  tags: ['autodocs'],
} satisfies Meta<typeof CollaborationView>;

export default meta;
type Story = StoryObj<typeof meta>;

const defaultSession: CollaborationSession = {
  id: 'session-1',
  title: 'Multi-Agent Research Session',
  agents: [
    { id: 'a1', name: 'Research Agent', status: 'working', role: 'Researcher' },
    { id: 'a2', name: 'Analysis Agent', status: 'thinking', role: 'Analyst' },
    { id: 'a3', name: 'Writer Agent', status: 'idle', role: 'Writer' },
  ],
  messages: [
    { id: 'm1', from: 'user', to: 'a1', content: 'Research the latest AI trends' },
    { id: 'm2', from: 'a1', to: 'a2', content: 'Found 15 relevant papers', kind: 'message' },
    { id: 'm3', from: 'a2', content: 'Analyzing trends...', kind: 'tool_call' },
  ],
  pendingApprovals: [],
};

export const Default: Story = {
  args: { session: defaultSession },
};

export const WithPendingApprovals: Story = {
  args: {
    session: {
      ...defaultSession,
      pendingApprovals: [
        { id: 'ap1', agentId: 'a1', title: 'Access external API', detail: 'Requires approval' },
      ],
    },
  },
};

export const EmptySession: Story = {
  args: {
    session: {
      id: 'empty',
      agents: [],
      messages: [],
      pendingApprovals: [],
    },
  },
};
