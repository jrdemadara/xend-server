# Intimacy System (Planned)

## Overview
The intimacy feature is designed as a private, progression-based relationship space system.

- A user can belong to multiple intimacy spaces.
- An intimacy space can have more than two members.
- Members can have multiple spaces with different people/groups.
- Progression unlocks features as intimacy level increases.

## Finalized v1 Scope

### Daily Connection Quest (Ritual)
- This is a **daily ritual**, not a game.
- One quest is generated per space per day.
- Goal is to strengthen relationships through consistent small actions.
- Full reward/streak is granted when all required members complete the ritual.

### Truth or Dare (Only Game)
- `truth_or_dare` is the only game in v1.
- Prompt selection is based on the space's current intimacy level.
- Lower levels use lighter prompts; higher levels can unlock deeper/intimate prompts.

## Product Goals
- Support long-distance relationship bonding.
- Encourage healthy interaction via shared goals.
- Unlock deeper experiences over time based on mutual participation.
- Keep everything privacy-first and member-only.

## Core Concepts

### 1) Relationship Spaces
Private groups where members interact, progress levels, and unlock features.

### 2) Intimacy Levels
Each space has a current level.

- Higher levels unlock more intimate experiences.
- Level-up requires all required members to meet level requirements.

### 3) Level Requirements
Each level has conditions, for example:

- Daily ritual completion count
- Participation/streak requirements
- Specific event completions (e.g., Truth or Dare participation)

### 4) Daily Connection Quest
Daily prompts to strengthen relationships, for example:

- Send a cute picture in the morning
- Send a motivating audio/video message
- Send a quick emotional check-in

Quest completion is tracked per space and per member.

### 5) Truth or Dare
- Turn-based game session in a private space.
- Prompt pool is filtered by intimacy level.
- Participation contributes to level progression signals (based on product rule tuning).

### 6) Relationship Mood
Members can set mood states in a space (e.g., happy, angry, anxious).

- Mood should be modeled as events over time, not a single static value.
- Space can derive current mood from latest member updates.

## Suggested Data Model (Draft)
- `relationship_spaces`
- `relationship_space_members`
- `intimacy_levels`
- `space_level_state`
- `level_requirements`
- `space_requirement_progress`
- `daily_connection_quests`
- `daily_connection_quest_submissions`
- `truth_or_dare_sessions`
- `truth_or_dare_turns`
- `truth_or_dare_actions`
- `relationship_moods`

## Suggested Rules (Draft)
- A user cannot invite themselves.
- Invite and membership are explicit consent-based.
- Access is strictly limited to active space members.
- Level-up is atomic and validated server-side.
- Truth or Dare prompts must respect current intimacy level.
- Highly intimate unlocks can require additional explicit consent.

## Privacy Considerations
- Keep sensitive relationship data minimal and encrypted where applicable.
- Apply strict authorization checks for all space resources.
- Support deletion/retention controls per policy.
- Avoid unnecessary analytics on intimate interactions.

## Status
Planned only. Not yet implemented in DB migrations or API.
