/**
 * CollaborationReplay.stories.tsx — CollaborationReplay 组件 Storybook stories。
 */

import type { Meta, StoryObj } from '@storybook/react';
import { CollaborationReplay } from '../collaboration/CollaborationReplay.js';

const meta = {
  title: 'Collaboration/CollaborationReplay',
  component: CollaborationReplay,
  tags: ['autodocs'],
} satisfies Meta<typeof CollaborationReplay>;

export default meta;
type Story = StoryObj<typeof meta>;

const sampleAgents = [
  { id: 'a1', name: 'Research Agent', status: 'done' as const },
  { id: 'a2', name: 'Writer Agent', status: 'done' as const },
];

const sampleMessages = [
  { id: 'm1', from: 'user', to: 'a1', content: 'Please research AI trends' },
  { id: 'm2', from: 'a1', content: 'Searching for papers...', kind: 'tool_call' as const },
  { id: 'm3', from: 'a1', content: 'Found 15 papers on the topic', kind: 'tool_result' as const },
  { id: 'm4', from: 'a1', to: 'a2', content: 'Here are the key findings for you to summarize' },
  { id: 'm5', from: 'a2', content: 'Generating summary...' },
  { id: 'm6', from: 'a2', to: 'user', content: 'Summary: AI is advancing rapidly in 3 key areas.' },
];

export const Default: Story = {
  args: {
    messages: sampleMessages,
    agents: sampleAgents,
    intervalMs: 800,
  },
};

export const FastPlayback: Story = {
  args: {
    messages: sampleMessages,
    agents: sampleAgents,
    intervalMs: 300,
  },
};

export const Empty: Story = {
  args: {
    messages: [],
    agents: [],
  },
};

export const SingleMessage: Story = {
  args: {
    messages: [{ id: 'm1', from: 'user', content: 'Hello' }],
    agents: sampleAgents,
  },
};
