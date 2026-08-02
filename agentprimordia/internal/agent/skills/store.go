package skills

import "sync"

// Store 技能库：技能存取（内存实现，生产环境可对接 SQLite）
type Store struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewStore 创建技能库
func NewStore() *Store {
	return &Store{
		skills: make(map[string]*Skill),
	}
}

// Save 保存技能
func (s *Store) Save(skill *Skill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skills[skill.ID] = skill
}

// Get 获取技能
func (s *Store) Get(id string) (*Skill, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.skills[id]
	return sk, ok
}

// Delete 删除技能
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.skills, id)
}

// List 列出所有技能
func (s *Store) List() []*Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		result = append(result, sk)
	}
	return result
}

// ListActive 列出所有活跃技能
func (s *Store) ListActive() []*Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Skill
	for _, sk := range s.skills {
		if sk.Status == SkillActive {
			result = append(result, sk)
		}
	}
	return result
}

// FindByName 按名称查找
func (s *Store) FindByName(name string) (*Skill, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sk := range s.skills {
		if sk.Name == name {
			return sk, true
		}
	}
	return nil, false
}

// Count 返回技能总数
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.skills)
}
