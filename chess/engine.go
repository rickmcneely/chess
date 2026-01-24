package chess

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	Empty = iota
	WhitePawn
	WhiteRook
	WhiteKnight
	WhiteBishop
	WhiteQueen
	WhiteKing
	BlackPawn
	BlackRook
	BlackKnight
	BlackBishop
	BlackQueen
	BlackKing
)

type Board struct {
	Squares         [8][8]int
	WhiteToMove     bool
	CastlingRights  string
	EnPassantSquare string
	HalfMoveClock   int
	FullMoveNumber  int
}

type Move struct {
	From      string
	To        string
	Promotion string
}

func NewBoard() *Board {
	return ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
}

func ParseFEN(fen string) *Board {
	board := &Board{}
	parts := strings.Split(fen, " ")

	rows := strings.Split(parts[0], "/")
	for r, row := range rows {
		col := 0
		for _, ch := range row {
			if unicode.IsDigit(ch) {
				col += int(ch - '0')
			} else {
				board.Squares[r][col] = charToPiece(ch)
				col++
			}
		}
	}

	if len(parts) > 1 {
		board.WhiteToMove = parts[1] == "w"
	} else {
		board.WhiteToMove = true
	}

	if len(parts) > 2 {
		board.CastlingRights = parts[2]
	} else {
		board.CastlingRights = "KQkq"
	}

	if len(parts) > 3 {
		board.EnPassantSquare = parts[3]
	} else {
		board.EnPassantSquare = "-"
	}

	if len(parts) > 4 {
		fmt.Sscanf(parts[4], "%d", &board.HalfMoveClock)
	}

	if len(parts) > 5 {
		fmt.Sscanf(parts[5], "%d", &board.FullMoveNumber)
	} else {
		board.FullMoveNumber = 1
	}

	return board
}

func (b *Board) ToFEN() string {
	var fen strings.Builder

	for r := 0; r < 8; r++ {
		empty := 0
		for c := 0; c < 8; c++ {
			if b.Squares[r][c] == Empty {
				empty++
			} else {
				if empty > 0 {
					fen.WriteString(fmt.Sprintf("%d", empty))
					empty = 0
				}
				fen.WriteRune(pieceToChar(b.Squares[r][c]))
			}
		}
		if empty > 0 {
			fen.WriteString(fmt.Sprintf("%d", empty))
		}
		if r < 7 {
			fen.WriteString("/")
		}
	}

	if b.WhiteToMove {
		fen.WriteString(" w ")
	} else {
		fen.WriteString(" b ")
	}

	if b.CastlingRights == "" {
		fen.WriteString("-")
	} else {
		fen.WriteString(b.CastlingRights)
	}

	fen.WriteString(" ")
	fen.WriteString(b.EnPassantSquare)
	fen.WriteString(fmt.Sprintf(" %d %d", b.HalfMoveClock, b.FullMoveNumber))

	return fen.String()
}

func charToPiece(ch rune) int {
	switch ch {
	case 'P':
		return WhitePawn
	case 'R':
		return WhiteRook
	case 'N':
		return WhiteKnight
	case 'B':
		return WhiteBishop
	case 'Q':
		return WhiteQueen
	case 'K':
		return WhiteKing
	case 'p':
		return BlackPawn
	case 'r':
		return BlackRook
	case 'n':
		return BlackKnight
	case 'b':
		return BlackBishop
	case 'q':
		return BlackQueen
	case 'k':
		return BlackKing
	}
	return Empty
}

func pieceToChar(piece int) rune {
	switch piece {
	case WhitePawn:
		return 'P'
	case WhiteRook:
		return 'R'
	case WhiteKnight:
		return 'N'
	case WhiteBishop:
		return 'B'
	case WhiteQueen:
		return 'Q'
	case WhiteKing:
		return 'K'
	case BlackPawn:
		return 'p'
	case BlackRook:
		return 'r'
	case BlackKnight:
		return 'n'
	case BlackBishop:
		return 'b'
	case BlackQueen:
		return 'q'
	case BlackKing:
		return 'k'
	}
	return ' '
}

func squareToCoords(sq string) (int, int) {
	if len(sq) != 2 {
		return -1, -1
	}
	col := int(sq[0] - 'a')
	row := 8 - int(sq[1]-'0')
	return row, col
}

func coordsToSquare(row, col int) string {
	return fmt.Sprintf("%c%d", 'a'+col, 8-row)
}

func isWhitePiece(piece int) bool {
	return piece >= WhitePawn && piece <= WhiteKing
}

func isBlackPiece(piece int) bool {
	return piece >= BlackPawn && piece <= BlackKing
}

func (b *Board) IsValidMove(move Move) bool {
	fromRow, fromCol := squareToCoords(move.From)
	toRow, toCol := squareToCoords(move.To)

	if fromRow < 0 || fromRow > 7 || fromCol < 0 || fromCol > 7 {
		return false
	}
	if toRow < 0 || toRow > 7 || toCol < 0 || toCol > 7 {
		return false
	}

	piece := b.Squares[fromRow][fromCol]
	if piece == Empty {
		return false
	}

	if b.WhiteToMove && !isWhitePiece(piece) {
		return false
	}
	if !b.WhiteToMove && !isBlackPiece(piece) {
		return false
	}

	target := b.Squares[toRow][toCol]
	if target != Empty {
		if b.WhiteToMove && isWhitePiece(target) {
			return false
		}
		if !b.WhiteToMove && isBlackPiece(target) {
			return false
		}
	}

	if !b.isPseudoLegalMove(piece, fromRow, fromCol, toRow, toCol, move) {
		return false
	}

	testBoard := b.Copy()
	testBoard.makeMove(move)
	// Check if the move leaves our own king in check (not allowed)
	if testBoard.IsInCheck(b.WhiteToMove) {
		return false
	}

	return true
}

func (b *Board) isPseudoLegalMove(piece, fromRow, fromCol, toRow, toCol int, move Move) bool {
	switch piece {
	case WhitePawn:
		return b.isValidWhitePawnMove(fromRow, fromCol, toRow, toCol, move)
	case BlackPawn:
		return b.isValidBlackPawnMove(fromRow, fromCol, toRow, toCol, move)
	case WhiteRook, BlackRook:
		return b.isValidRookMove(fromRow, fromCol, toRow, toCol)
	case WhiteKnight, BlackKnight:
		return b.isValidKnightMove(fromRow, fromCol, toRow, toCol)
	case WhiteBishop, BlackBishop:
		return b.isValidBishopMove(fromRow, fromCol, toRow, toCol)
	case WhiteQueen, BlackQueen:
		return b.isValidQueenMove(fromRow, fromCol, toRow, toCol)
	case WhiteKing, BlackKing:
		return b.isValidKingMove(fromRow, fromCol, toRow, toCol)
	}
	return false
}

func (b *Board) isValidWhitePawnMove(fromRow, fromCol, toRow, toCol int, move Move) bool {
	if fromCol == toCol {
		if toRow == fromRow-1 && b.Squares[toRow][toCol] == Empty {
			return true
		}
		if fromRow == 6 && toRow == 4 && b.Squares[5][toCol] == Empty && b.Squares[4][toCol] == Empty {
			return true
		}
	}

	if abs(toCol-fromCol) == 1 && toRow == fromRow-1 {
		if isBlackPiece(b.Squares[toRow][toCol]) {
			return true
		}
		if b.EnPassantSquare == coordsToSquare(toRow, toCol) {
			return true
		}
	}

	return false
}

func (b *Board) isValidBlackPawnMove(fromRow, fromCol, toRow, toCol int, move Move) bool {
	if fromCol == toCol {
		if toRow == fromRow+1 && b.Squares[toRow][toCol] == Empty {
			return true
		}
		if fromRow == 1 && toRow == 3 && b.Squares[2][toCol] == Empty && b.Squares[3][toCol] == Empty {
			return true
		}
	}

	if abs(toCol-fromCol) == 1 && toRow == fromRow+1 {
		if isWhitePiece(b.Squares[toRow][toCol]) {
			return true
		}
		if b.EnPassantSquare == coordsToSquare(toRow, toCol) {
			return true
		}
	}

	return false
}

func (b *Board) isValidRookMove(fromRow, fromCol, toRow, toCol int) bool {
	if fromRow != toRow && fromCol != toCol {
		return false
	}
	return b.isPathClear(fromRow, fromCol, toRow, toCol)
}

func (b *Board) isValidKnightMove(fromRow, fromCol, toRow, toCol int) bool {
	rowDiff := abs(toRow - fromRow)
	colDiff := abs(toCol - fromCol)
	return (rowDiff == 2 && colDiff == 1) || (rowDiff == 1 && colDiff == 2)
}

func (b *Board) isValidBishopMove(fromRow, fromCol, toRow, toCol int) bool {
	if abs(toRow-fromRow) != abs(toCol-fromCol) {
		return false
	}
	return b.isPathClear(fromRow, fromCol, toRow, toCol)
}

func (b *Board) isValidQueenMove(fromRow, fromCol, toRow, toCol int) bool {
	return b.isValidRookMove(fromRow, fromCol, toRow, toCol) ||
		b.isValidBishopMove(fromRow, fromCol, toRow, toCol)
}

func (b *Board) isValidKingMove(fromRow, fromCol, toRow, toCol int) bool {
	rowDiff := abs(toRow - fromRow)
	colDiff := abs(toCol - fromCol)

	if rowDiff <= 1 && colDiff <= 1 {
		return true
	}

	if rowDiff == 0 && colDiff == 2 {
		return b.isValidCastling(fromRow, fromCol, toRow, toCol)
	}

	return false
}

func (b *Board) isValidCastling(fromRow, fromCol, toRow, toCol int) bool {
	if b.IsInCheck(b.WhiteToMove) {
		return false
	}

	if b.WhiteToMove {
		if fromRow != 7 || fromCol != 4 {
			return false
		}
		if toCol == 6 {
			if !strings.Contains(b.CastlingRights, "K") {
				return false
			}
			if b.Squares[7][5] != Empty || b.Squares[7][6] != Empty {
				return false
			}
			if b.Squares[7][7] != WhiteRook {
				return false
			}
			testBoard := b.Copy()
			testBoard.Squares[7][5] = WhiteKing
			testBoard.Squares[7][4] = Empty
			if testBoard.IsInCheck(true) {
				return false
			}
			return true
		}
		if toCol == 2 {
			if !strings.Contains(b.CastlingRights, "Q") {
				return false
			}
			if b.Squares[7][1] != Empty || b.Squares[7][2] != Empty || b.Squares[7][3] != Empty {
				return false
			}
			if b.Squares[7][0] != WhiteRook {
				return false
			}
			testBoard := b.Copy()
			testBoard.Squares[7][3] = WhiteKing
			testBoard.Squares[7][4] = Empty
			if testBoard.IsInCheck(true) {
				return false
			}
			return true
		}
	} else {
		if fromRow != 0 || fromCol != 4 {
			return false
		}
		if toCol == 6 {
			if !strings.Contains(b.CastlingRights, "k") {
				return false
			}
			if b.Squares[0][5] != Empty || b.Squares[0][6] != Empty {
				return false
			}
			if b.Squares[0][7] != BlackRook {
				return false
			}
			testBoard := b.Copy()
			testBoard.Squares[0][5] = BlackKing
			testBoard.Squares[0][4] = Empty
			if testBoard.IsInCheck(false) {
				return false
			}
			return true
		}
		if toCol == 2 {
			if !strings.Contains(b.CastlingRights, "q") {
				return false
			}
			if b.Squares[0][1] != Empty || b.Squares[0][2] != Empty || b.Squares[0][3] != Empty {
				return false
			}
			if b.Squares[0][0] != BlackRook {
				return false
			}
			testBoard := b.Copy()
			testBoard.Squares[0][3] = BlackKing
			testBoard.Squares[0][4] = Empty
			if testBoard.IsInCheck(false) {
				return false
			}
			return true
		}
	}

	return false
}

func (b *Board) isPathClear(fromRow, fromCol, toRow, toCol int) bool {
	rowDir := sign(toRow - fromRow)
	colDir := sign(toCol - fromCol)

	r, c := fromRow+rowDir, fromCol+colDir
	for r != toRow || c != toCol {
		if b.Squares[r][c] != Empty {
			return false
		}
		r += rowDir
		c += colDir
	}
	return true
}

func (b *Board) Copy() *Board {
	newBoard := &Board{
		WhiteToMove:     b.WhiteToMove,
		CastlingRights:  b.CastlingRights,
		EnPassantSquare: b.EnPassantSquare,
		HalfMoveClock:   b.HalfMoveClock,
		FullMoveNumber:  b.FullMoveNumber,
	}
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			newBoard.Squares[r][c] = b.Squares[r][c]
		}
	}
	return newBoard
}

func (b *Board) makeMove(move Move) {
	fromRow, fromCol := squareToCoords(move.From)
	toRow, toCol := squareToCoords(move.To)

	piece := b.Squares[fromRow][fromCol]

	if (piece == WhitePawn || piece == BlackPawn) && move.To == b.EnPassantSquare {
		if piece == WhitePawn {
			b.Squares[toRow+1][toCol] = Empty
		} else {
			b.Squares[toRow-1][toCol] = Empty
		}
	}

	b.EnPassantSquare = "-"
	if piece == WhitePawn && fromRow == 6 && toRow == 4 {
		b.EnPassantSquare = coordsToSquare(5, fromCol)
	} else if piece == BlackPawn && fromRow == 1 && toRow == 3 {
		b.EnPassantSquare = coordsToSquare(2, fromCol)
	}

	if piece == WhiteKing && fromCol == 4 && toCol == 6 {
		b.Squares[7][5] = WhiteRook
		b.Squares[7][7] = Empty
	} else if piece == WhiteKing && fromCol == 4 && toCol == 2 {
		b.Squares[7][3] = WhiteRook
		b.Squares[7][0] = Empty
	} else if piece == BlackKing && fromCol == 4 && toCol == 6 {
		b.Squares[0][5] = BlackRook
		b.Squares[0][7] = Empty
	} else if piece == BlackKing && fromCol == 4 && toCol == 2 {
		b.Squares[0][3] = BlackRook
		b.Squares[0][0] = Empty
	}

	b.Squares[toRow][toCol] = piece
	b.Squares[fromRow][fromCol] = Empty

	if move.Promotion != "" {
		promotionPiece := charToPiece(rune(move.Promotion[0]))
		if b.WhiteToMove && toRow == 0 {
			b.Squares[toRow][toCol] = promotionPiece
		} else if !b.WhiteToMove && toRow == 7 {
			b.Squares[toRow][toCol] = promotionPiece
		}
	} else {
		if piece == WhitePawn && toRow == 0 {
			b.Squares[toRow][toCol] = WhiteQueen
		} else if piece == BlackPawn && toRow == 7 {
			b.Squares[toRow][toCol] = BlackQueen
		}
	}

	if piece == WhiteKing {
		b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "K", "")
		b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "Q", "")
	} else if piece == BlackKing {
		b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "k", "")
		b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "q", "")
	} else if piece == WhiteRook {
		if fromRow == 7 && fromCol == 7 {
			b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "K", "")
		} else if fromRow == 7 && fromCol == 0 {
			b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "Q", "")
		}
	} else if piece == BlackRook {
		if fromRow == 0 && fromCol == 7 {
			b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "k", "")
		} else if fromRow == 0 && fromCol == 0 {
			b.CastlingRights = strings.ReplaceAll(b.CastlingRights, "q", "")
		}
	}

	if b.CastlingRights == "" {
		b.CastlingRights = "-"
	}

	if !b.WhiteToMove {
		b.FullMoveNumber++
	}
	b.WhiteToMove = !b.WhiteToMove
}

func (b *Board) MakeMove(move Move) *Board {
	newBoard := b.Copy()
	newBoard.makeMove(move)
	return newBoard
}

func (b *Board) IsInCheck(whiteKing bool) bool {
	kingRow, kingCol := -1, -1
	kingPiece := WhiteKing
	if !whiteKing {
		kingPiece = BlackKing
	}

	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if b.Squares[r][c] == kingPiece {
				kingRow, kingCol = r, c
				break
			}
		}
		if kingRow >= 0 {
			break
		}
	}

	if kingRow < 0 {
		return false
	}

	return b.isSquareAttacked(kingRow, kingCol, !whiteKing)
}

func (b *Board) isSquareAttacked(row, col int, byWhite bool) bool {
	if byWhite {
		if row < 7 {
			if col > 0 && b.Squares[row+1][col-1] == WhitePawn {
				return true
			}
			if col < 7 && b.Squares[row+1][col+1] == WhitePawn {
				return true
			}
		}
	} else {
		if row > 0 {
			if col > 0 && b.Squares[row-1][col-1] == BlackPawn {
				return true
			}
			if col < 7 && b.Squares[row-1][col+1] == BlackPawn {
				return true
			}
		}
	}

	knightMoves := [][2]int{{-2, -1}, {-2, 1}, {-1, -2}, {-1, 2}, {1, -2}, {1, 2}, {2, -1}, {2, 1}}
	knightPiece := BlackKnight
	if byWhite {
		knightPiece = WhiteKnight
	}
	for _, m := range knightMoves {
		r, c := row+m[0], col+m[1]
		if r >= 0 && r < 8 && c >= 0 && c < 8 && b.Squares[r][c] == knightPiece {
			return true
		}
	}

	rookPiece, queenPiece := BlackRook, BlackQueen
	if byWhite {
		rookPiece, queenPiece = WhiteRook, WhiteQueen
	}
	for _, dir := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		r, c := row+dir[0], col+dir[1]
		for r >= 0 && r < 8 && c >= 0 && c < 8 {
			piece := b.Squares[r][c]
			if piece != Empty {
				if piece == rookPiece || piece == queenPiece {
					return true
				}
				break
			}
			r += dir[0]
			c += dir[1]
		}
	}

	bishopPiece := BlackBishop
	if byWhite {
		bishopPiece = WhiteBishop
	}
	for _, dir := range [][2]int{{-1, -1}, {-1, 1}, {1, -1}, {1, 1}} {
		r, c := row+dir[0], col+dir[1]
		for r >= 0 && r < 8 && c >= 0 && c < 8 {
			piece := b.Squares[r][c]
			if piece != Empty {
				if piece == bishopPiece || piece == queenPiece {
					return true
				}
				break
			}
			r += dir[0]
			c += dir[1]
		}
	}

	kingPiece := BlackKing
	if byWhite {
		kingPiece = WhiteKing
	}
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr == 0 && dc == 0 {
				continue
			}
			r, c := row+dr, col+dc
			if r >= 0 && r < 8 && c >= 0 && c < 8 && b.Squares[r][c] == kingPiece {
				return true
			}
		}
	}

	return false
}

func (b *Board) GetLegalMoves() []Move {
	var moves []Move

	for fromRow := 0; fromRow < 8; fromRow++ {
		for fromCol := 0; fromCol < 8; fromCol++ {
			piece := b.Squares[fromRow][fromCol]
			if piece == Empty {
				continue
			}
			if b.WhiteToMove && !isWhitePiece(piece) {
				continue
			}
			if !b.WhiteToMove && !isBlackPiece(piece) {
				continue
			}

			for toRow := 0; toRow < 8; toRow++ {
				for toCol := 0; toCol < 8; toCol++ {
					from := coordsToSquare(fromRow, fromCol)
					to := coordsToSquare(toRow, toCol)
					move := Move{From: from, To: to}

					if b.IsValidMove(move) {
						if (piece == WhitePawn && toRow == 0) || (piece == BlackPawn && toRow == 7) {
							for _, promo := range []string{"Q", "R", "B", "N"} {
								if !b.WhiteToMove {
									promo = strings.ToLower(promo)
								}
								moves = append(moves, Move{From: from, To: to, Promotion: promo})
							}
						} else {
							moves = append(moves, move)
						}
					}
				}
			}
		}
	}

	return moves
}

func (b *Board) IsCheckmate() bool {
	if !b.IsInCheck(b.WhiteToMove) {
		return false
	}
	return len(b.GetLegalMoves()) == 0
}

func (b *Board) IsStalemate() bool {
	if b.IsInCheck(b.WhiteToMove) {
		return false
	}
	return len(b.GetLegalMoves()) == 0
}

func (b *Board) GetGameStatus() string {
	if b.IsCheckmate() {
		return "checkmate"
	}
	if b.IsStalemate() {
		return "stalemate"
	}
	if b.IsInCheck(b.WhiteToMove) {
		return "check"
	}
	return "active"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

func ParseMove(moveStr string) Move {
	moveStr = strings.TrimSpace(moveStr)
	if len(moveStr) < 4 {
		return Move{}
	}

	move := Move{
		From: moveStr[0:2],
		To:   moveStr[2:4],
	}

	if len(moveStr) > 4 {
		move.Promotion = string(moveStr[4])
	}

	return move
}
