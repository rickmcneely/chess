package chess

import (
	"math/rand"
	"time"
)

// AIEngine represents a chess AI with configurable difficulty
type AIEngine struct {
	Depth int // Search depth (1=easy, 2=medium, 3=hard, 4=expert)
}

// Piece values for evaluation
const (
	PawnValue   = 100
	KnightValue = 320
	BishopValue = 330
	RookValue   = 500
	QueenValue  = 900
	KingValue   = 20000
)

// Piece-square tables for positional evaluation (from white's perspective)
// Higher values = better squares for that piece

var pawnTable = [8][8]int{
	{0, 0, 0, 0, 0, 0, 0, 0},
	{50, 50, 50, 50, 50, 50, 50, 50},
	{10, 10, 20, 30, 30, 20, 10, 10},
	{5, 5, 10, 25, 25, 10, 5, 5},
	{0, 0, 0, 20, 20, 0, 0, 0},
	{5, -5, -10, 0, 0, -10, -5, 5},
	{5, 10, 10, -20, -20, 10, 10, 5},
	{0, 0, 0, 0, 0, 0, 0, 0},
}

var knightTable = [8][8]int{
	{-50, -40, -30, -30, -30, -30, -40, -50},
	{-40, -20, 0, 0, 0, 0, -20, -40},
	{-30, 0, 10, 15, 15, 10, 0, -30},
	{-30, 5, 15, 20, 20, 15, 5, -30},
	{-30, 0, 15, 20, 20, 15, 0, -30},
	{-30, 5, 10, 15, 15, 10, 5, -30},
	{-40, -20, 0, 5, 5, 0, -20, -40},
	{-50, -40, -30, -30, -30, -30, -40, -50},
}

var bishopTable = [8][8]int{
	{-20, -10, -10, -10, -10, -10, -10, -20},
	{-10, 0, 0, 0, 0, 0, 0, -10},
	{-10, 0, 5, 10, 10, 5, 0, -10},
	{-10, 5, 5, 10, 10, 5, 5, -10},
	{-10, 0, 10, 10, 10, 10, 0, -10},
	{-10, 10, 10, 10, 10, 10, 10, -10},
	{-10, 5, 0, 0, 0, 0, 5, -10},
	{-20, -10, -10, -10, -10, -10, -10, -20},
}

var rookTable = [8][8]int{
	{0, 0, 0, 0, 0, 0, 0, 0},
	{5, 10, 10, 10, 10, 10, 10, 5},
	{-5, 0, 0, 0, 0, 0, 0, -5},
	{-5, 0, 0, 0, 0, 0, 0, -5},
	{-5, 0, 0, 0, 0, 0, 0, -5},
	{-5, 0, 0, 0, 0, 0, 0, -5},
	{-5, 0, 0, 0, 0, 0, 0, -5},
	{0, 0, 0, 5, 5, 0, 0, 0},
}

var queenTable = [8][8]int{
	{-20, -10, -10, -5, -5, -10, -10, -20},
	{-10, 0, 0, 0, 0, 0, 0, -10},
	{-10, 0, 5, 5, 5, 5, 0, -10},
	{-5, 0, 5, 5, 5, 5, 0, -5},
	{0, 0, 5, 5, 5, 5, 0, -5},
	{-10, 5, 5, 5, 5, 5, 0, -10},
	{-10, 0, 5, 0, 0, 0, 0, -10},
	{-20, -10, -10, -5, -5, -10, -10, -20},
}

var kingMiddleGameTable = [8][8]int{
	{-30, -40, -40, -50, -50, -40, -40, -30},
	{-30, -40, -40, -50, -50, -40, -40, -30},
	{-30, -40, -40, -50, -50, -40, -40, -30},
	{-30, -40, -40, -50, -50, -40, -40, -30},
	{-20, -30, -30, -40, -40, -30, -30, -20},
	{-10, -20, -20, -20, -20, -20, -20, -10},
	{20, 20, 0, 0, 0, 0, 20, 20},
	{20, 30, 10, 0, 0, 10, 30, 20},
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewAIEngine creates a new AI engine with the specified depth
func NewAIEngine(depth int) *AIEngine {
	if depth < 1 {
		depth = 1
	}
	if depth > 4 {
		depth = 4
	}
	return &AIEngine{Depth: depth}
}

// GetBestMove returns the best move for the current position
func (ai *AIEngine) GetBestMove(board *Board) Move {
	moves := board.GetLegalMoves()
	if len(moves) == 0 {
		return Move{}
	}

	// Shuffle moves to add randomness when multiple moves have the same score
	rand.Shuffle(len(moves), func(i, j int) {
		moves[i], moves[j] = moves[j], moves[i]
	})

	var bestMove Move
	bestScore := -1000000

	alpha := -1000000
	beta := 1000000

	for _, move := range moves {
		newBoard := board.MakeMove(move)
		// Negate because we're looking from opponent's perspective
		score := -ai.minimax(newBoard, ai.Depth-1, -beta, -alpha, false)

		if score > bestScore {
			bestScore = score
			bestMove = move
		}
		if score > alpha {
			alpha = score
		}
	}

	return bestMove
}

// minimax with alpha-beta pruning
func (ai *AIEngine) minimax(board *Board, depth int, alpha, beta int, maximizing bool) int {
	if depth == 0 {
		return ai.evaluatePosition(board)
	}

	moves := board.GetLegalMoves()

	// Check for terminal positions
	if len(moves) == 0 {
		if board.IsInCheck(board.WhiteToMove) {
			// Checkmate - return large negative value (bad for current player)
			return -100000 - depth // Prefer faster checkmates
		}
		// Stalemate
		return 0
	}

	if maximizing {
		maxScore := -1000000
		for _, move := range moves {
			newBoard := board.MakeMove(move)
			score := ai.minimax(newBoard, depth-1, alpha, beta, false)
			if score > maxScore {
				maxScore = score
			}
			if score > alpha {
				alpha = score
			}
			if beta <= alpha {
				break // Beta cutoff
			}
		}
		return maxScore
	} else {
		minScore := 1000000
		for _, move := range moves {
			newBoard := board.MakeMove(move)
			score := ai.minimax(newBoard, depth-1, alpha, beta, true)
			if score < minScore {
				minScore = score
			}
			if score < beta {
				beta = score
			}
			if beta <= alpha {
				break // Alpha cutoff
			}
		}
		return minScore
	}
}

// evaluatePosition returns a score for the position from white's perspective
func (ai *AIEngine) evaluatePosition(board *Board) int {
	score := 0

	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			piece := board.Squares[r][c]
			if piece == Empty {
				continue
			}

			pieceValue := 0
			positionBonus := 0

			switch piece {
			case WhitePawn:
				pieceValue = PawnValue
				positionBonus = pawnTable[r][c]
			case BlackPawn:
				pieceValue = -PawnValue
				positionBonus = -pawnTable[7-r][c]
			case WhiteKnight:
				pieceValue = KnightValue
				positionBonus = knightTable[r][c]
			case BlackKnight:
				pieceValue = -KnightValue
				positionBonus = -knightTable[7-r][c]
			case WhiteBishop:
				pieceValue = BishopValue
				positionBonus = bishopTable[r][c]
			case BlackBishop:
				pieceValue = -BishopValue
				positionBonus = -bishopTable[7-r][c]
			case WhiteRook:
				pieceValue = RookValue
				positionBonus = rookTable[r][c]
			case BlackRook:
				pieceValue = -RookValue
				positionBonus = -rookTable[7-r][c]
			case WhiteQueen:
				pieceValue = QueenValue
				positionBonus = queenTable[r][c]
			case BlackQueen:
				pieceValue = -QueenValue
				positionBonus = -queenTable[7-r][c]
			case WhiteKing:
				pieceValue = KingValue
				positionBonus = kingMiddleGameTable[r][c]
			case BlackKing:
				pieceValue = -KingValue
				positionBonus = -kingMiddleGameTable[7-r][c]
			}

			score += pieceValue + positionBonus
		}
	}

	// Return score from current player's perspective
	if board.WhiteToMove {
		return score
	}
	return -score
}

// MoveToString converts a Move to algebraic notation (e.g., "e2e4")
func MoveToString(move Move) string {
	result := move.From + move.To
	if move.Promotion != "" {
		result += move.Promotion
	}
	return result
}
